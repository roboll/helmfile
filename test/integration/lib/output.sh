#!/usr/bin/env bash

declare -i tests_total=0

function info () {
    tput bold; tput setaf 4; echo -n "INFO: "; tput sgr0; echo "${@}"
}
function warn () {
    tput bold; tput setaf 3; echo -n "WARN: "; tput sgr0; echo "${@}"
}
function fail () {
    tput bold; tput setaf 1; echo -n "FAIL: "; tput sgr0; echo "${@}"
    exit 1
}
function test_start () {
    tput bold; tput setaf 6; echo -n "TEST: "; tput sgr0; echo "${@}"
}
function test_pass () {
    tests_total=$((tests_total+1))
    tput bold; tput setaf 2; echo -n "PASS: "; tput sgr0; echo "${@}"
}
function all_tests_passed () {
    tput bold; tput setaf 2; echo -n "PASS: "; tput sgr0; echo "${tests_total} tests passed"
}
