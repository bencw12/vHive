// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ease-lab/vhive/metrics"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	parallelNum      = flag.Int("parallel", 1, "Number of parallel instances to start")
	iterNum          = flag.Int("iter", 1, "Number of iterations to run")
	sleepBeforeTrace = flag.Bool("sleepBeforeTrace", false, "Whether to sleep after taking the snapshot but before the trace")
	sleepBeforeTest  = flag.Bool("sleepBeforeTest", false, "Whether to sleep after the trace but before the experiment")
	sleepTimeSec     = flag.Int("sleepTimeSec", 10, "Seconds to sleep (only applies if sleepBeforeTrace | sleepBeforeTest is set")
	funcName         = flag.String("funcName", "helloworld", "Name of the function to benchmark")
	funcPath         = flag.String("funcPath", "", "Path of locally build container image for function to use in benchmark")
	resetTrace       = flag.Bool("resetTrace", false, "whether to reset the trace for stability tests")
	doMemTrace       = flag.Bool("memTrace", false, "whether to report provenance of each guest memory page on restore")
	funcArgs         = flag.String("funcArgs", "{}", "JSON string of arguments passed to the function being executed")
)

func TestBenchParallelServe(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		serveMetrics       = make([]*metrics.Metric, *parallelNum)
		upfMetrics         = make([]*metrics.Metric, *parallelNum)
		images             = getAllImages()
		parallel           = *parallelNum
		vmID               = 0
		concurrency        = 2
	)

	imageName, isPresent := images[*funcName]
	require.True(t, isPresent, "Function is not supported")

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	// Pull image
	_, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, *funcArgs, true, true)
	require.NoError(t, err, "Function returned error")
	// require.Equal(t, resp.Payload, "Hello, record_response!")

	createSnapshots(t, concurrency, vmID, imageName, isSyncOffload)
	log.Info("All snapshots created")

	createRecords(t, concurrency, vmID, imageName, isSyncOffload)
	log.Info("All records done")

	// Measure
	var vmGroup sync.WaitGroup

	if !*isWithCache {
		dropPageCache()
	}

	tStart := time.Now()
	for i := 0; i < parallel; i++ {
		vmIDString := strconv.Itoa(vmID + i)

		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()

			_, metr, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, false)
			require.NoError(t, err, "Function returned error")
			// require.Equal(t, resp.Payload, "Hello, replay_response!")

			serveMetrics[i] = metr
		}(i)
	}

	vmGroup.Wait()

	log.Printf("Started %d instances in %d milliseconds", parallel, time.Since(tStart).Milliseconds())

	for i := 0; i < parallel; i++ {
		vmIDString := strconv.Itoa(vmID + i)
		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		if *isUPFEnabledTest {
			memManagerMetrics, err := orch.GetUPFLatencyStats(vmIDString + "_0")
			require.NoError(t, err, "Failed to ge tupf metrics")
			require.Equal(t, len(memManagerMetrics), 1, "wrong length")
			upfMetrics[i] = memManagerMetrics[0]
		}
	}

	fusePrintMetrics(t, serveMetrics, upfMetrics, isUPFEnabledTest, true, *funcName, "parallelServe.csv")

}

func TestBenchWarmServe(t *testing.T) {
	var (
		servedTh          uint64
		pinnedFuncNum     int
		isSyncOffload     bool = true
		images                 = getAllImages()
		vmID                   = 0
		memManagerMetrics []*metrics.Metric
	)

	imageName, isPresent := images[*funcName]
	require.True(t, isPresent, "Function is not supported")

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	vmIDString := strconv.Itoa(vmID)

	// First time invoke (cold start)
	_, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, true)
	require.NoError(t, err, "Function returned error")
	// require.Equal(t, resp.Payload, "Hello, replay_response!")

	// memory footprint
	memFootprint, err := getMemFootprint()
	if err != nil {
		log.Warnf("Failed to get memory footprint of VM=%s, image=%s\n", vmIDString, imageName)
	}

	// Measure warm
	serveMetrics := make([]*metrics.Metric, *iterNum)

	for k := 0; k < *iterNum; k++ {
		if !*isWithCache {
			dropPageCache()
		}

		_, met, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		serveMetrics[k] = met
	}

	message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
	require.NoError(t, err, "Function returned error, "+message)

	// FUSE
	if orch.GetUPFEnabled() {
		// Page stats
		err = funcPool.DumpUPFPageStats(vmIDString, imageName, *funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+*funcName)

		memManagerMetrics, err = orch.GetUPFLatencyStats(vmIDString + "_0")
		require.NoError(t, err, "Failed to dump get stats for "+*funcName)
		require.Equal(t, len(serveMetrics), len(memManagerMetrics), "different metrics lengths")
	}

	fusePrintMetrics(t, serveMetrics, memManagerMetrics, isUPFEnabledTest, true, *funcName, "serve.csv")

	appendMemFootprint(getOutFile("serve.csv"), memFootprint)

}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir() // Ensure it's not a directory
}

func TestWSStability(t *testing.T) {
	var (
		servedTh          uint64
		pinnedFuncNum     int
		isSyncOffload     bool = true
		vmID                   = 0
		memManagerMetrics []*metrics.Metric
	)

	log.Infof("testing funcPath %s", *funcPath)

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	_, _, err := funcPool.Serve(context.Background(), "plr_fnc", *funcPath, *funcArgs, true, true)

	require.NoError(t, err, "Function returned error")
	// require.Equal(t, resp.Payload, "Hello, record_response!")

	vmIDString := strconv.Itoa(vmID)

	log.Info("Creating snapshots...")

	createSnapshots(t, 1, vmID, *funcPath, isSyncOffload)

	// Measure
	serveMetrics := make([]*metrics.Metric, *iterNum)

	// full experiment with prefetching
	for k := 0; k < *iterNum; k++ {
		log.Info("Doing memory trace...")
		createRecords(t, 1, vmID, *funcPath, isSyncOffload)

		if !*isWithCache {
			dropPageCache()
		}

		_, met, err := funcPool.Serve(context.Background(), vmIDString, *funcPath, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		// copy restore log to results dir
		restoreLog := fmt.Sprintf("restore_%v.log", k)

		if *resetTrace {
			restoreLog = fmt.Sprintf("restore_reset_%v.log", k)
		}

		cmd := exec.Command("cp", "/tmp/log_0_0_after.logs", getOutFile(restoreLog))
		if err = cmd.Run(); err != nil {
			log.Fatalf("Failed to copy restore log: %v", err)
		}

		cmd = exec.Command("chmod", "a+r", getOutFile(restoreLog))
		if err = cmd.Run(); err != nil {
			log.Fatalf("Failed to set permissions on restore log: %v", err)
		}

		serveMetrics[k] = met

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging

		err = funcPool.DumpUPFPageStats(vmIDString, *funcPath, *funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+*funcName)
		if *resetTrace {
			funcPool.ResetTrace(vmIDString, *funcPath)
		}
	}

	// FUSE
	if orch.GetUPFEnabled() {
		err = funcPool.DumpUPFPageStats(vmIDString, *funcPath, *funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+*funcName)
		memManagerMetrics, err = orch.GetUPFLatencyStats(vmIDString + "_0")
		require.NoError(t, err, "Failed to dump stats for "+*funcName)
	}

	servePath := "serve.csv"

	fusePrintMetrics(t, serveMetrics, memManagerMetrics, isUPFEnabledTest, true, *funcName, servePath)

	// do the memory trace
	// TODO print working set AND all faults outside the working set, should be easy
	if *doMemTrace && *resetTrace {
		// hack
		err = os.Remove("/tmp/fc-mem.log")
		if err != nil {
			fmt.Println("Error removing guest memory trace file: ", err)
		}
		require.NoError(t, err, "Failed to remove fc-mem.log")

		if !*isWithCache {
			dropPageCache()
		}

		_, _, err := funcPool.Serve(context.Background(), vmIDString, *funcPath, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging
		cmd := exec.Command("cp", "/tmp/fc-mem.log", getOutFile("mem-trace.log"))
		if err = cmd.Run(); err != nil {
			log.Fatalf("Failed to copy memory trace: %v", err)
		}
	}

	// run again without prefaulting and track working set accuracy
	// this will use the most recent trace
	// for k := 0; k < *iterNum; k++ {
	// 	if !*isWithCache {
	// 		dropPageCache()
	// 	}

	// 	resp, _, err := funcPool.Serve(context.Background(), vmIDString, *funcPath, "replay", false)
	// 	require.NoError(t, err, "Function returned error")
	// 	require.Equal(t, resp.Payload, "Hello, replay_response!")

	// 	time.Sleep(1 * time.Second) // this helps kworker hanging

	// 	message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
	// 	require.NoError(t, err, "Function returned error, "+message)

	// 	time.Sleep(3 * time.Second) // this helps kworker hanging
	// }

	// servePath = "wsAccuracy.csv"

	// if *resetTrace {
	// 	servePath = "wsAccuracyReset.csv"
	// }
	// // FUSE
	// if orch.GetUPFEnabled() {
	// 	// Page stats
	// 	err = funcPool.DumpUPFPageStats(
	// 		vmIDString,
	// 		*funcPath,
	// 		*funcName,
	// 		getOutFile(servePath))

	// 	require.NoError(t, err, "Failed to dump page stats for"+*funcName)
	// }
}

func TestBenchLocalServe(t *testing.T) {
	var (
		servedTh          uint64
		pinnedFuncNum     int
		isSyncOffload     bool = true
		vmID                   = 0
		memManagerMetrics []*metrics.Metric
	)

	// require.True(t, fileExists(*funcPath), "container path does not exist %s", *funcPath)
	log.Infof("testing funcPath %s", *funcPath)

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	_, _, err := funcPool.Serve(context.Background(), "plr_fnc", *funcPath, *funcArgs, true, true)

	require.NoError(t, err, "Function returned error")
	// require.Equal(t, resp.Payload, "Hello, record_response!")

	vmIDString := strconv.Itoa(vmID)

	log.Info("Creating snapshots...")

	createSnapshots(t, 1, vmID, *funcPath, isSyncOffload)

	log.Info("Doing memory trace...")
	createRecords(t, 1, vmID, *funcPath, isSyncOffload)

	// Measure
	serveMetrics := make([]*metrics.Metric, *iterNum)

	// full experiment with prefetching
	for k := 0; k < *iterNum; k++ {
		if !*isWithCache {
			dropPageCache()
		}

		_, met, err := funcPool.Serve(context.Background(), vmIDString, *funcPath, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		serveMetrics[k] = met

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging
	}

	// run again without prefaulting and track working set accuracy
	for k := 0; k < *iterNum; k++ {
		if !*isWithCache {
			dropPageCache()
		}

		_, _, err := funcPool.Serve(context.Background(), vmIDString, *funcPath, *funcArgs, false, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging
	}

	// do memory trace (after everything else so we don't disturb results)
	if orch.GetUPFEnabled() {
		// hacky but remove last memory trace
		err = os.Remove("/tmp/fc-mem.log")
		if err != nil {
			fmt.Println("Error removing file: ", err)
		}
		require.NoError(t, err, "Failed to remove fc-mem.log")

		if !*isWithCache {
			dropPageCache()
		}

		_, _, err = funcPool.Serve(context.Background(), vmIDString, *funcPath, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, *funcPath, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging

		memTracePath := fmt.Sprintf("%s/fc-mem.log", *benchDir)
		cmd := exec.Command("cp", "/tmp/fc-mem.log", memTracePath)
		_, err = cmd.Output()
		if err != nil {
			log.Info("Failed to copy trace")
			panic("firecracker memory trace failed")
		}
	}
	log.Info("Finished tracing")

	// FUSE
	if orch.GetUPFEnabled() {
		// Page stats
		err = funcPool.DumpUPFPageStats(vmIDString, *funcPath, *funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+*funcName)

		memManagerMetrics, err = orch.GetUPFLatencyStats(vmIDString + "_0")
		require.NoError(t, err, "Failed to dump get stats for "+*funcName)
		// require.Equal(t, len(serveMetrics), len(memManagerMetrics), "different metrics lengths") // BCWH FIX
	}

	fusePrintMetrics(t, serveMetrics, memManagerMetrics, isUPFEnabledTest, true, *funcName, "serve.csv")
}

func TestBenchServe(t *testing.T) {
	var (
		servedTh          uint64
		pinnedFuncNum     int
		isSyncOffload     bool = true
		images                 = getAllImages()
		vmID                   = 0
		memManagerMetrics []*metrics.Metric
	)

	imageName, isPresent := images[*funcName]
	require.True(t, isPresent, "Function is not supported")

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()
	// Pull image
	_, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, *funcArgs, true, true)
	require.NoError(t, err, "Function returned error")

	vmIDString := strconv.Itoa(vmID)

	log.Info("Creating snapshots...")

	createSnapshots(t, 1, vmID, imageName, isSyncOffload)

	if *sleepBeforeTrace {
		time.Sleep(time.Duration(*sleepTimeSec) * time.Second)
	}

	log.Info("Doing memory trace...")
	createRecords(t, 1, vmID, imageName, isSyncOffload)
	log.Info("Finished tracing")

	// Measure
	serveMetrics := make([]*metrics.Metric, *iterNum)

	if *sleepBeforeTest {
		time.Sleep(time.Duration(*sleepTimeSec) * time.Second)
	}

	// full experiment with prefetching
	for k := 0; k < *iterNum; k++ {
		if !*isWithCache {
			dropPageCache()
		}

		_, met, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		serveMetrics[k] = met

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging
	}

	// run again without prefaulting and track working set accuracy
	for k := 0; k < *iterNum; k++ {
		if !*isWithCache {
			dropPageCache()
		}

		_, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, false, false)
		require.NoError(t, err, "Function returned error")
		// require.Equal(t, resp.Payload, "Hello, replay_response!")

		time.Sleep(1 * time.Second) // this helps kworker hanging

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		time.Sleep(3 * time.Second) // this helps kworker hanging
	}

	// FUSE
	if orch.GetUPFEnabled() {
		// Page stats
		err = funcPool.DumpUPFPageStats(vmIDString, imageName, *funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+*funcName)

		memManagerMetrics, err = orch.GetUPFLatencyStats(vmIDString + "_0")
		require.NoError(t, err, "Failed to dump get stats for "+*funcName)
		// require.Equal(t, len(serveMetrics), len(memManagerMetrics), "different metrics lengths") // BCWH FIX
	}

	fusePrintMetrics(t, serveMetrics, memManagerMetrics, isUPFEnabledTest, true, *funcName, "serve.csv")

}

// /////////////////////////////////////////////////////////////////////////////
// //////////////////////// Auxialiary functions below /////////////////////////
// /////////////////////////////////////////////////////////////////////////////
func fusePrintMetrics(t *testing.T, serveMetrics, upfMetrics []*metrics.Metric, isUPFEnabledTest *bool, printIndiv bool, funcName, outfile string) {
	outFileName := getOutFile(outfile)

	if *isUPFEnabledTest {
		for i, metr := range serveMetrics {
			for k, v := range upfMetrics[i].MetricMap {
				metr.MetricMap[k] = v
			}
		}
	}

	if printIndiv {
		for _, metr := range serveMetrics {
			err := metrics.PrintMeanStd(outFileName, funcName, metr)
			require.NoError(t, err, "Failed to dump stats")
		}
	}

	err := metrics.PrintMeanStd(outFileName, funcName, serveMetrics...)
	require.NoError(t, err, "Failed to dump stats")
}

func createSnapshots(t *testing.T, concurrency, vmID int, imageName string, isSyncOffload bool) {
	parallel := *parallelNum
	sem := make(chan bool, concurrency)

	for i := 0; i < parallel; i++ {
		vmIDString := strconv.Itoa(vmID + i)

		sem <- true

		go func(vmIDString string) {
			defer func() { <-sem }()

			// Create VM (and snapshot)
			_, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, true)
			require.NoError(t, err, "Function returned error")
			// require.Equal(t, resp.Payload, "Hello, record_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}(vmIDString)
	}

	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
}

func createRecords(t *testing.T, concurrency, vmID int, imageName string, isSyncOffload bool) {
	parallel := *parallelNum
	sem := make(chan bool, concurrency)

	for i := 0; i < parallel; i++ {
		vmIDString := strconv.Itoa(vmID + i)

		sem <- true

		go func(vmIDString string) {
			defer func() { <-sem }()

			// Record
			_, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, *funcArgs, true, false)
			require.NoError(t, err, "Function returned error")
			// require.Equal(t, resp.Payload, "Hello, record_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}(vmIDString)
	}

	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
}

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}

func createResultsDir() {
	if err := os.MkdirAll(*benchDir, 0777); err != nil {
		log.Fatalf("Failed to create results dir: %v", err)
	}
}

func getOutFile(name string) string {
	return filepath.Join(*benchDir, name)
}

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld":          "vhiveease/helloworld:var_workload",
		"chameleon":           "vhiveease/chameleon:var_workload",
		"pyaes":               "vhiveease/pyaes:var_workload",
		"image_rotate":        "vhiveease/image_rotate:var_workload",
		"image_rotate_s3":     "vhiveease/image_rotate_s3:var_workload",
		"json_serdes":         "vhiveease/json_serdes:var_workload",
		"json_serdes_s3":      "vhiveease/json_serdes_s3:var_workload",
		"lr_serving":          "vhiveease/lr_serving:var_workload",
		"cnn_serving":         "vhiveease/cnn_serving:var_workload",
		"rnn_serving":         "vhiveease/rnn_serving:var_workload",
		"lr_training_s3":      "vhiveease/lr_training_s3:var_workload",
		"lr_training":         "vhiveease/lr_training:var_workload",
		"video_processing_s3": "vhiveease/video_processing_s3:var_workload",
	}
}

// getFirecrackerPid Assumes that only one Firecracker process is running
func getFirecrackerPid() ([]byte, error) {
	pidBytes, err := exec.Command("pidof", "firecracker").Output()
	if err != nil {
		log.Warnf("Failed to get Firecracker PID: %v", err)
	}

	return pidBytes, err
}

func getMemFootprint() (float64, error) {
	pidBytes, err := getFirecrackerPid()
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(pidBytes), "\n")[0]))
	if err != nil {
		log.Warnf("Pid conversion failed: %v", err)
	}

	cmd := exec.Command("ps", "-o", "rss", "-p", strconv.Itoa(pid))

	stdout, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to run ps command: %v", err)
	}

	infoArr := strings.Split(string(stdout), "\n")[1]
	stats := strings.Fields(infoArr)

	rss, err := strconv.ParseFloat(stats[0], 64)
	if err != nil {
		log.Warnf("Error in conversion when computing rss: %v", err)
		return 0, err
	}

	rss *= 1024

	return rss, nil
}

func appendMemFootprint(outFileName string, memFootprint float64) {
	f, err := os.OpenFile(outFileName, os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf("MemFootprint\t%12.1f\n", memFootprint)); err != nil {
		log.Println(err)
	}
}
