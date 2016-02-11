#!/usr/bin/env python

import subprocess
import argparse
import re

RE_ROTATION = re.compile(r'Rotation\s*:\s*(\d+)')

parser = argparse.ArgumentParser(description='Transcode a video')

parser.add_argument('in_file', help='Input file')
parser.add_argument('out_file', help='Output file')

ARGS = parser.parse_args()

# Parse the rotation metadata from the file
metadata = subprocess.check_output(['exiftool', '-Rotation', ARGS.in_file])
rotation_match = RE_ROTATION.search(metadata)
if not rotation_match:
    raise RuntimeError('Rotation metadata not found')

rotation = int(rotation_match.group(1))

print('Found rotation: {}'.format(rotation))

cmd = ['avconv']

# Input file
cmd += ['-i', ARGS.in_file]

# Automatically overwrite
cmd += ['-y']

# Convert audio: copy
cmd += ['-c:a', 'copy']

# Convert video: h264
cmd += ['-c:v', 'h264']

# Baseline profile for compatability
cmd += ['-profile:v', 'baseline']

# Better quality
cmd += ['-qscale', '1']

# Compensation for rotation
ROTATION_ARGUMENTS = {
    0: [],
    90: ['-vf', 'transpose=1'],
    180: ['-vf', 'vflip,hflip'],
    270: ['-vf', 'transpose=3'],
}
cmd += ROTATION_ARGUMENTS.get(rotation, [])

# Output file
cmd += [ARGS.out_file]

print('> {}'.format(' '.join(cmd)))

# Actually call avconv
subprocess.check_call(cmd)

