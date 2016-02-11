#!/usr/bin/env python

import os
import time
import subprocess
from collections import namedtuple
import argparse

def safe_mkdir(path):
    try:
        os.mkdir(path)
        return True
    except OSError:
        return False

def safe_rename(src, dst):
    try:
        os.rename(src, dst)
        return True
    except OSError:
        return False

def safe_remove(path):
    try:
        os.remove(path)
        return True
    except OSError:
        return False

parser = argparse.ArgumentParser(description='Watch a folder and transcode videos')

parser.add_argument('in_dir', help='Input path')
parser.add_argument('work_dir', help='Intermediate work path')
parser.add_argument('out_dir', help='Output path')
parser.add_argument('--delay', help='Delay between in seconds iterations', type=float, default=1.0)
parser.add_argument('--maxprocs', help='Number of transcoding processes that can be active', type=int, default=32)

ARGS = parser.parse_args()

# Create the intermediate work directories
work_src = os.path.join(ARGS.work_dir, 'src')
work_dst = os.path.join(ARGS.work_dir, 'dst')

safe_mkdir(work_src)
safe_mkdir(work_dst)

# Expect transcoder.py to be in the same directory as self
working_directory = os.getcwd()
transcoder = os.path.join(working_directory, 'transcode.py')

TranscodeProc = namedtuple('TranscodeProc', 'proc src dst out')

# Currently active transcoding processes
transcode_procs = []

while True:

    # Collect pending work
    pending_files = os.listdir(ARGS.in_dir)
    for pending_file in pending_files:

        # Ignore hidden files
        if pending_file.startswith('.'):
            continue

        # Don't begin work if there are no resources
        if len(transcode_procs) >= ARGS.maxprocs:
            break

        pending_path = os.path.join(ARGS.in_dir, pending_file)
        src_path = os.path.join(work_src, pending_file)
        dst_path = os.path.join(work_dst, pending_file)
        done_path = os.path.join(ARGS.out_dir, pending_file)

        # Move the file to the transcode source path
        if not safe_rename(pending_path, src_path):
            continue

        # Spawn the transcoding process
        args = [transcoder, src_path, dst_path]
        process = subprocess.Popen(args)

        transcode_procs.append(TranscodeProc(proc=process,
            src=src_path, dst=dst_path, out=done_path))

    # Move the finished transcoded files to the output directory and remove
    # them from the list
    still_active = []
    for transcode_proc in transcode_procs:
        return_code = transcode_proc.proc.poll()

        # Check if still running
        if return_code is None:
            still_active.append(transcode_proc)
            continue

        if return_code == 0:
            # Success: delete the source file and move the destination to the
            # output folder
            safe_rename(transcode_proc.dst, transcode_proc.out)
            safe_remove(transcode_proc.src)
        else:
            # Failed: Delete both files
            safe_remove(transcode_proc.src)
            safe_remove(transcode_proc.dst)

    # Keep the still active procs
    transcode_procs = still_active

    # Yield before looping again
    time.sleep(ARGS.delay)

