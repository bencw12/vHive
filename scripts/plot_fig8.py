#!/usr/bin/python3

import os
import matplotlib.pyplot as plt

ROOT_DIR = os.path.split(os.path.realpath(__file__))[0] + "/.."
RESULTS_DIR = ROOT_DIR + "/all_results"
FUNCTIONS = ROOT_DIR + "/scripts/functions.txt"

BASELINE_INVOCATION = 7
BASELINE_VMM = 9
BASELINE_CONN = 3

REAP_INVOCATION = 9
REAP_VMM = 13
REAP_CONN = 3

COLORS = {
    'vmm': 'tab:blue',
    'conn': 'tab:orange',
    'func': 'tab:green'
}

WIDTH=0.2

def get_func_list():
    with open(FUNCTIONS) as f:
        funcs = f.readlines()
        funcs = [x.split('\n')[0] for x in funcs]
        return funcs

def plot_one_func(ax, f, idx):
    baseline = open(RESULTS_DIR + f"/baseline/{f}/serve.csv")\
        .readlines()[-1].split('\n')[0].split(',')

    # confirmed these are the correct values
    func_time = int(baseline[BASELINE_INVOCATION])
    conn_time = int(baseline[BASELINE_CONN])
    vmm_time = int(baseline[BASELINE_VMM])
    total=int((func_time + conn_time + vmm_time) / 1000)
    
    ax.bar(idx, vmm_time, color=COLORS['vmm'], width=WIDTH)
    ax.bar(idx, conn_time, bottom=vmm_time, color=COLORS['conn'], width=WIDTH)
    ax.bar(idx, func_time, bottom=conn_time + vmm_time, color=COLORS['func'], width=WIDTH)
    ax.annotate(f"{total}", (idx, (total * 1000) + 1000), ha='center')

    reap = open(RESULTS_DIR + f"/reap/{f}/serve.csv")\
        .readlines()[-1].split('\n')[0].split(',')

    func_time = int(reap[REAP_INVOCATION])
    conn_time = int(reap[REAP_CONN])
    vmm_time = int(reap[REAP_VMM])
    total=int((func_time + conn_time + vmm_time) / 1000)

    ax.bar(idx + (WIDTH + WIDTH/2), vmm_time, color=COLORS['vmm'], width=WIDTH)
    ax.bar(idx + (WIDTH + WIDTH/2), conn_time, bottom=vmm_time, color=COLORS['conn'], width=WIDTH)
    ax.bar(idx + (WIDTH + WIDTH/2), func_time, bottom=conn_time + vmm_time, color=COLORS['func'], width=WIDTH)
    ax.annotate(f"{total}", (idx + (WIDTH + WIDTH/2), (total * 1000) + 1000), ha='center')


def plot_all():
    # fix figsize
    idx = 0
    fig, ax = plt.subplots(1, 1)
    funcs = get_func_list()
    for f in funcs:
        plot_one_func(ax, f, idx)
        idx += 1

    ticks = []
    labels = []

    idx = 0
    for f in funcs:
        labels.append(f)
        labels.append(f"{f} REAP")

        ticks.append(idx)
        ticks.append(idx + (WIDTH + WIDTH/2))
        
        idx += 1

    plt.xticks(ticks, labels, rotation=50, ha='right')
    plt.tight_layout()
    
    plt.savefig(ROOT_DIR + "/graph.pdf")

if __name__ == '__main__':
    plot_all()
