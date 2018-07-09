#!/bin/sh

set -e

if [ ! -f "build/env.sh" ]; then
    echo "$0 must be run from the root of the repository."
    exit 2
fi

# Create fake Go workspace if it doesn't exist yet.
workspace="$PWD/build/_workspace"
root="$PWD"
letdir="$workspace/src/github.com/linkeye"
if [ ! -L "$letdir/linkeye" ]; then
    mkdir -p "$letdir"
    cd "$letdir"
    ln -s ../../../../../. linkeye
    cd "$root"
fi

# Set up the environment to use the workspace.
GOPATH="$workspace"
export GOPATH

# Run the command inside the workspace.
cd "$letdir/linkeye"
PWD="$letdir/linkeye"

# Launch the arguments with the configured environment.
exec "$@"
