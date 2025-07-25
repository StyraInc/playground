#!/usr/bin/env bash

PLAY_DIR=$(
    dir=$(dirname "${BASH_SOURCE}")/..
    cd "$dir"
    pwd
)
source $PLAY_DIR/scripts/utils.sh


function play::check_lint() {
    exec 5>&1
    exit_code=0
    for pkg in $(play::go_packages); do
        __output=$(golint $pkg | tee >(cat - >&5))
        if [ ! -z "$__output" ]; then
            exit_code=1
        fi
    done
    exit $exit_code
}

play::check_lint
