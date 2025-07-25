#!/usr/bin/env bash

PLAY_DIR=$(dirname "${BASH_SOURCE}")/..
source $PLAY_DIR/scripts/utils.sh

function play::check_fmt() {
    exec 5>&1
    exit_code=0
    for pkg in $(play::go_packages); do
        for file in $(play::go_files_in_package $pkg); do
            __diff=$(gofmt -d $file | tee >(cat - >&5))
            if [ ! -z "$__diff" ]; then
                exit_code=1
            fi
        done
    done
    exit $exit_code
}

play::check_fmt
