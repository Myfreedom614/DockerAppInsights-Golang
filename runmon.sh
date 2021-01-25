#!/bin/bash
cd ~/dockerappinsights/
./insmon -ti 30 > /dev/null 2>&1 &
disown
jobs -l
ps -aux | grep insmon
