// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package manager

/*
#include "user_page_faults.h"
*/
import "C"

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/ease-lab/vhive/metrics"

	"unsafe"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID string

	VMMStatePath, GuestMemPath, WorkingSetPath string

	InstanceSockAddr string
	BaseDir          string // base directory for the instance
	MetricsPath      string // path to csv file where the metrics should be stored
	IsLazyMode       bool
	GuestMemSize     int
	metricsModeOn    bool
}

type MemRange struct {
	start uint64
	len   uint64
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	firstPageFaultOnce *sync.Once // to initialize the start virtual address and replay
	startAddress       uint64
	userFaultFD        *os.File
	trace              *Trace
	epfd               int
	quitCh             chan int
	scanCh             chan int

	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isRecordReady bool

	guestMem    []byte
	workingSet  []byte
	kernelPhdrs []MemRange

	// Stats
	totalPFServed       []float64
	uniquePFServed      []float64
	reusedPFServed      []float64
	zeroPFServedWS      []float64
	zeroPFServedUnique  []float64
	kernelPFServedInWS  []float64
	kernelPFServedOutWS []float64
	latencyMetrics      []*metrics.Metric
	inWSPFServed        []float64
	fetchStateTimes     []float64

	replayedNum    int    // only valid for lazy serving
	zeroNumWS      uint64 // number of zero pages from WS given to the guest in lazy mode
	zeroNumUnique  uint64 // number of unique zero pages given to the guest in lazy mode
	uniqueNum      int
	kernelNumInWS  int // number of faults intercepted from kernel pages in the WS
	kernelNumOutWS int // number of faults intercepted from kernel pages outside the WS
	currentMetric  *metrics.Metric

	inWS         uint64
	prefault     bool
	uniquePFList [][]uint64
	uniquePF     []uint64
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.SnapshotStateCfg = cfg

	s.trace = initTrace(s.getTraceFile())
	if s.metricsModeOn {
		s.totalPFServed = make([]float64, 0)
		s.uniquePFServed = make([]float64, 0)
		s.inWSPFServed = make([]float64, 0)
		s.reusedPFServed = make([]float64, 0)
		s.latencyMetrics = make([]*metrics.Metric, 0)
		s.kernelPhdrs = make([]MemRange, 0)
		s.kernelPFServedInWS = make([]float64, 0)
		s.kernelPFServedOutWS = make([]float64, 0)
		s.uniquePFList = make([][]uint64, 0, 0)
		s.uniquePF = make([]uint64, 0)
		s.zeroPFServedWS = make([]float64, 0)
		s.zeroPFServedUnique = make([]float64, 0)
		s.fetchStateTimes = make([]float64, 0)

		// TODO don't hardcode elf path
		// this is where the kernel is, though
		file, err := os.Open("/fast/bcwh/git/junction/lib/reap/bin/vmlinux.bin")
		if err != nil {
			panic(fmt.Sprintf("failed to open kernel ELF: %v", err))
		}

		defer file.Close()

		elf, err := elf.NewFile(file)
		if err != nil {
			panic(fmt.Sprintf("failed to parse ELF: %v", err))
		}

		for _, prog := range elf.Progs {
			if prog.Type == 1 {
				s.kernelPhdrs = append(s.kernelPhdrs, MemRange{start: prog.Paddr, len: prog.Memsz})
			}
		}
	}

	return s
}

func (s *SnapshotState) setupStateOnActivate() {
	s.isActive = true
	s.isEverActivated = true
	s.firstPageFaultOnce = new(sync.Once)
	s.quitCh = make(chan int)
	s.scanCh = make(chan int)

	if s.metricsModeOn {
		s.uniqueNum = 0
		s.replayedNum = 0
		s.currentMetric = metrics.NewMetric()
		s.inWS = 0
		s.kernelNumInWS = 0
		s.kernelNumOutWS = 0
		s.zeroNumWS = 0
		s.zeroNumUnique = 0
		s.uniquePF = make([]uint64, 0)
	}
}

func (s *SnapshotState) getUFFD() error {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", s.InstanceSockAddr)
		if err != nil {
			if ctx.Err() != nil {
				log.Error("Failed to dial within the context timeout")
				return err
			}
			time.Sleep(1 * time.Millisecond)
			continue
		}

		defer c.Close()

		sendfdConn := c.(*net.UnixConn)

		fs, err := fd.Get(sendfdConn, 1, []string{"a file"})
		if err != nil {
			log.Error("Failed to receive the uffd")
			return err
		}

		s.userFaultFD = fs[0]

		return nil
	}
}

func (s *SnapshotState) processMetrics() {
	if s.metricsModeOn && s.isRecordReady {

		if s.prefault {
			s.uniquePFServed = append(s.uniquePFServed, float64(s.uniqueNum))
			s.kernelPFServedOutWS = append(s.kernelPFServedOutWS, float64(s.kernelNumOutWS))
			s.uniquePFList = append(s.uniquePFList, s.uniquePF)
			s.zeroPFServedWS = append(s.zeroPFServedWS, float64(s.zeroNumWS))
			s.zeroPFServedUnique = append(s.zeroPFServedUnique, float64(s.zeroNumUnique))
		}

		if !s.prefault {
			s.inWSPFServed = append(s.inWSPFServed, float64(s.inWS))
			s.kernelPFServedInWS = append(s.kernelPFServedInWS, float64(s.kernelNumInWS))
		}

		if s.IsLazyMode {
			s.totalPFServed = append(s.totalPFServed, float64(s.replayedNum))
			s.reusedPFServed = append(
				s.reusedPFServed,
				float64(s.replayedNum-s.uniqueNum),
			)
		}

		s.latencyMetrics = append(s.latencyMetrics, s.currentMetric)
	}
}

func (s *SnapshotState) getTraceFile() string {
	return filepath.Join(s.BaseDir, "trace")
}

func (s *SnapshotState) mapGuestMemory() error {
	fd, err := os.OpenFile(s.GuestMemPath, os.O_RDONLY, 0444)
	if err != nil {
		log.Errorf("Failed to open guest memory file: %v", err)
		return err
	}

	s.guestMem, err = unix.Mmap(int(fd.Fd()), 0, s.GuestMemSize, unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		log.Errorf("Failed to mmap guest memory file: %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) unmapGuestMemory() error {
	<-s.scanCh
	if err := unix.Munmap(s.guestMem); err != nil {
		log.Errorf("Failed to munmap guest memory file: %v", err)
		return err
	}

	return nil
}

// alignment returns alignment of the block in memory
// with reference to alignSize
//
// Can't check alignment of a zero sized block as &block[0] is invalid
func alignment(block []byte, alignSize int) int {
	return int(uintptr(unsafe.Pointer(&block[0])) & uintptr(alignSize-1))
}

// AlignedBlock returns []byte of size BlockSize aligned to a multiple
// of alignSize in memory (must be power of two)
func AlignedBlock(blockSize int) []byte {
	alignSize := os.Getpagesize() // must be multiple of the filesystem block size

	if blockSize == 0 {
		return nil
	}

	block := make([]byte, blockSize+alignSize)

	a := alignment(block, alignSize)
	offset := 0
	if a != 0 {
		offset = alignSize - a
	}
	block = block[offset : offset+blockSize]

	// Check
	if blockSize != 0 {
		a = alignment(block, alignSize)
		if a != 0 {
			log.Fatal("Failed to align block")
		}
	}
	return block
}

// fetchState Fetches the working set file (or the whole guest memory) and the VMM state file
func (s *SnapshotState) fetchState() error {
	tStart := time.Now()

	if _, err := ioutil.ReadFile(s.VMMStatePath); err != nil {
		log.Errorf("Failed to fetch VMM state: %v\n", err)
		return err
	}

	size := len(s.trace.trace) * os.Getpagesize()

	// O_DIRECT allows to fully leverage disk bandwidth by bypassing the OS page cache
	f, err := os.OpenFile(s.WorkingSetPath, os.O_RDONLY|syscall.O_DIRECT, 0600)
	if err != nil {
		log.Errorf("Failed to open the working set file for direct-io: %v\n", err)
		return err
	}

	s.workingSet = AlignedBlock(size) // direct io requires aligned buffer

	if n, err := f.Read(s.workingSet); n != size || err != nil {
		log.Errorf("Reading working set file failed: %v\n", err)
		return err
	}

	log.Debug("Fetched the entire working set")
	if err := f.Close(); err != nil {
		log.Errorf("Failed to close the working set file: %v\n", err)
		return err
	}

	s.fetchStateTimes = append(s.fetchStateTimes, metrics.ToUS(time.Since(tStart)))

	return nil
}

func (s *SnapshotState) pollUserPageFaults(readyCh chan int) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var events [1]syscall.EpollEvent

	s.registerEpoller()

	logger.Debug("Starting polling loop")

	defer syscall.Close(s.epfd)

	readyCh <- 0

	for {
		select {
		case <-s.quitCh:

			// collect zero page metrics
			s.countWSZeroPages()
			s.countUniqueZeroPages()

			logger.Info("Handler received a signal to quit")
			s.scanCh <- 0
			return
		default:
			nevents, err := syscall.EpollWait(s.epfd, events[:], -1)
			if err != nil && err != syscall.EINTR {
				logger.Fatalf("epoll_wait: %v", err)
				break
			}

			// skip and continue handling
			if err == syscall.EINTR {
				continue
			}

			if nevents < 1 {
				panic("Wrong number of events")
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)

				stateFd := int(s.userFaultFD.Fd())

				if fd != stateFd && stateFd != -1 {
					logger.Fatalf("Received event from unknown fd")
				}

				goMsg := make([]byte, sizeOfUFFDMsg())

				if nread, err := syscall.Read(fd, goMsg); err != nil || nread != len(goMsg) {
					if !errors.Is(err, syscall.EBADF) {
						log.Fatalf("Read uffd_msg failed: %v", err)
					}
					break
				}

				if event := uint8(goMsg[0]); event != uffdPageFault() {
					log.Fatal("Received wrong event type")
				}

				address := binary.LittleEndian.Uint64(goMsg[16:])

				if err := s.servePageFault(fd, address); err != nil {
					log.Fatalf("Failed to serve page fault")
				}
			}
		}
	}
}

func (s *SnapshotState) registerEpoller() error {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var (
		err   error
		event syscall.EpollEvent
		fdInt int
	)

	fdInt = int(s.userFaultFD.Fd())

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fdInt)

	s.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		logger.Errorf("Failed to create epoller %v", err)
		return err
	}

	if err := syscall.EpollCtl(
		s.epfd,
		syscall.EPOLL_CTL_ADD,
		fdInt,
		&event,
	); err != nil {
		logger.Errorf("Failed to subscribe VM %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) ResetTrace() {
	s.isRecordReady = false
	s.trace = initTrace(s.getTraceFile())
}

func (s *SnapshotState) servePageFault(fd int, address uint64) error {
	var (
		tStart              time.Time
		workingSetInstalled bool
	)

	s.firstPageFaultOnce.Do(
		func() {
			s.startAddress = address

			// bypass prefaulting to see how many faults resolve to the working set
			if !s.prefault {
				return
			}

			if s.isRecordReady && !s.IsLazyMode {
				if s.metricsModeOn {
					tStart = time.Now()
				}
				s.installWorkingSetPages(fd)
				if s.metricsModeOn {
					s.currentMetric.MetricMap[installWSMetric] = metrics.ToUS(time.Since(tStart))
				}

				workingSetInstalled = true
			}
		})

	if workingSetInstalled {
		log.Infof("Page fault after working set installed addr = 0x%x", address)
		return nil
	}

	offset := address - s.startAddress

	src := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(os.Getpagesize()) - 1))
	mode := uint64(0)

	rec := Record{
		offset: offset,
	}

	if !s.prefault {
		if s.trace.containsRecord(rec) {
			s.inWS += 1
			for _, m := range s.kernelPhdrs {
				if (offset >= m.start) && (offset < (m.start + m.len)) {
					s.kernelNumInWS++
				}
			}
		}
	}

	for _, m := range s.kernelPhdrs {
		if (offset >= m.start) && (offset < (m.start + m.len)) && !s.trace.containsRecord(rec) {
			s.kernelNumOutWS++
		}
	}

	if !s.isRecordReady {
		s.trace.AppendRecord(rec)
	} else {
		log.Debug("Serving a page that is missing from the working set")
	}

	if s.metricsModeOn {
		if s.isRecordReady {
			if s.IsLazyMode {
				if !s.trace.containsRecord(rec) && s.prefault {
					s.uniqueNum++
				}
				s.replayedNum++
			} else {
				s.uniquePF = append(s.uniquePF, offset)
				s.uniqueNum++
			}

		}

		tStart = time.Now()
	}

	err := installRegion(fd, src, dst, mode, 1)

	if s.metricsModeOn {
		s.currentMetric.MetricMap[serveUniqueMetric] += metrics.ToUS(time.Since(tStart))
	}

	return err
}

func (s *SnapshotState) installWorkingSetPages(fd int) {
	log.Info("Installing the working set pages")

	// build a list of sorted regions
	keys := make([]uint64, 0)
	for k := range s.trace.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var (
		srcOffset uint64
	)

	for _, offset := range keys {
		// map of offset to length
		regLength := s.trace.regions[offset]
		regAddress := s.startAddress + offset
		mode := uint64(C.const_UFFDIO_COPY_MODE_DONTWAKE)
		src := uint64(uintptr(unsafe.Pointer(&s.workingSet[srcOffset])))
		dst := regAddress

		installRegion(fd, src, dst, mode, uint64(regLength))

		srcOffset += uint64(regLength) * 4096
	}

	wake(fd, s.startAddress, os.Getpagesize())
}

func (s *SnapshotState) pageIsZero(addr uint64) bool {
	ptr := (*uint64)(unsafe.Pointer(uintptr(addr)))

	// loop through page
	var sum uint64

	sum = 0
	for i := addr; i < (addr + 4096/8); i++ {
		sum |= *ptr
		ptr = (*uint64)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + 8))
	}

	return (sum == 0)
}

func (s *SnapshotState) countUniqueZeroPages() {
	log.Info("Counting zero pages from outside the working set")

	for _, offset := range s.uniquePF {
		base := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
		if s.pageIsZero(base) {
			s.zeroNumUnique++
		}
	}
}

func (s *SnapshotState) countWSZeroPages() {
	log.Info("Counting zero pages in the working set file")

	// build a list of sorted regions
	keys := make([]uint64, 0)
	for k := range s.trace.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var (
		srcOffset uint64
	)

	srcOffset = 0
	for _, offset := range keys {
		// this gives the length of the trace region in pages
		regLength := s.trace.regions[offset]

		// log.Infof("WS offset = 0x%x, length = %v pages", srcOffset, regLength)

		for i := 0; i < regLength; i++ {
			// byte index into working set file
			base := uint64(uintptr(unsafe.Pointer(&s.workingSet[srcOffset])))
			if s.pageIsZero(base) {
				s.zeroNumWS++
			}
			// this assumes the WS file is a list of sorted regions
			// which is also done in installWorkingSetPages
			srcOffset += 4096
		}
	}
}

func installRegion(fd int, src, dst, mode, len uint64) error {
	cUC := C.struct_uffdio_copy{
		mode: C.ulonglong(mode),
		copy: 0,
		src:  C.ulonglong(src),
		dst:  C.ulonglong(dst),
		len:  C.ulonglong(uint64(os.Getpagesize()) * len),
	}

	err := ioctl(uintptr(fd), int(C.const_UFFDIO_COPY), unsafe.Pointer(&cUC))
	if err != nil {
		return err
	}

	return nil
}

func ioctl(fd uintptr, request int, argp unsafe.Pointer) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		fd,
		uintptr(request),
		// Note that the conversion from unsafe.Pointer to uintptr _must_
		// occur in the call expression.  See the package unsafe documentation
		// for more details.
		uintptr(argp),
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", fmt.Errorf("%d", int(errno)))
	}

	return nil
}

func wake(fd int, startAddress uint64, len int) {
	cUR := C.struct_uffdio_range{
		start: C.ulonglong(startAddress),
		len:   C.ulonglong(len),
	}

	err := ioctl(uintptr(fd), int(C.const_UFFDIO_WAKE), unsafe.Pointer(&cUR))
	if err != nil {
		log.Fatalf("ioctl failed: %v", err)
	}
}

func registerForUpf(startAddress []byte, len uint64) int {
	return int(C.register_for_upf(unsafe.Pointer(&startAddress[0]), C.ulong(len)))
}

func sizeOfUFFDMsg() int {
	return C.sizeof_struct_uffd_msg
}

func uffdPageFault() uint8 {
	return uint8(C.const_UFFD_EVENT_PAGEFAULT)
}
