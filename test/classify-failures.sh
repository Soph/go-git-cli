#!/bin/bash
#
# Classify failing tests into categories to distinguish CLI shim bugs
# from potential go-git library bugs.
#
# Categories:
#   unimplemented  - hits "not yet implemented" or "is not a git command"
#   exit-code      - command ran but wrong exit code
#   output-mismatch- output differs from expected (test_cmp / grep fail)
#   fatal-error    - go-git returns an unexpected fatal/panic
#   test-framework - git's own test infrastructure subtests
#   other          - uncategorized
#
# Usage:
#   ./cli/git-go/classify-failures.sh <git-dist-path> <cli-bin-path> [test-files...]

set -e

GIT_DIST="$(cd "$1" && pwd)"
CLI_BIN="$(cd "$2" && pwd)"
shift 2

TESTS="${@:-t0000-basic.sh t0001-init.sh t1006-cat-file.sh t1500-rev-parse.sh t3200-branch.sh t5510-fetch.sh t5516-fetch-push.sh t7004-tag.sh t7508-status.sh}"

# Counters
total_unimpl=0
total_exit=0
total_output=0
total_fatal=0
total_framework=0
total_other=0
total_pass=0

for t in $TESTS; do
    test_base="${t%.sh}"

    output=$(cd "$GIT_DIST/t" && \
        GIT_TEST_INSTALLED="$CLI_BIN" \
        bash "./$t" -v 2>&1) || true

    # Per-suite counters
    n_unimpl=0 n_exit=0 n_output=0 n_fatal=0 n_framework=0 n_other=0 n_pass=0

    current_num=""
    current_title=""
    is_fail=false
    block=""

    classify_block() {
        if ! $is_fail || [ -z "$current_num" ]; then
            if [ -n "$current_num" ] && ! $is_fail; then
                ((n_pass++)) || true
            fi
            return
        fi

        # 1. Unimplemented command
        if echo "$block" | grep -q "not yet implemented in go-git\|is not a git command"; then
            ((n_unimpl++)) || true
            return
        fi

        # 2. Test framework subtests (write_and_run_sub_test_lib_test, run_sub_test_lib_test)
        if echo "$block" | grep -q "write_and_run_sub_test_lib_test\|run_sub_test_lib_test\|check_sub_test_lib_test"; then
            ((n_framework++)) || true
            return
        fi

        # 3. Exit code mismatch
        if echo "$block" | grep -q "test_expect_code:.*command exited with\|test_must_fail: command succeeded"; then
            ((n_exit++)) || true
            return
        fi

        # 4. Output mismatch (test_cmp, diff, grep failures)
        if echo "$block" | grep -q "^--- \|^+++ \|^@@\|test_cmp\|^diff "; then
            ((n_output++)) || true
            return
        fi

        # 5. Fatal/panic from go-git
        if echo "$block" | grep -q "^fatal:\|^panic:\|repository does not exist\|runtime error"; then
            ((n_fatal++)) || true
            return
        fi

        # 6. Other
        ((n_other++)) || true
    }

    while IFS= read -r line; do
        if [[ "$line" =~ ^expecting\ (success|failure)\ of\ [0-9]+\.([0-9]+)\ \'(.*)\'  ]]; then
            classify_block
            current_num="${BASH_REMATCH[2]}"
            current_title="${BASH_REMATCH[3]}"
            is_fail=false
            block=""
        fi

        if [[ "$line" =~ ^not\ ok\  ]]; then
            is_fail=true
        fi

        block+="$line"$'\n'
    done <<< "$output"
    classify_block

    printf "%-25s pass:%-4d unimpl:%-3d exit:%-3d output:%-3d fatal:%-3d framework:%-3d other:%-3d\n" \
        "$t" "$n_pass" "$n_unimpl" "$n_exit" "$n_output" "$n_fatal" "$n_framework" "$n_other"

    total_unimpl=$((total_unimpl + n_unimpl))
    total_exit=$((total_exit + n_exit))
    total_output=$((total_output + n_output))
    total_fatal=$((total_fatal + n_fatal))
    total_framework=$((total_framework + n_framework))
    total_other=$((total_other + n_other))
    total_pass=$((total_pass + n_pass))
done

echo ""
printf "%-25s pass:%-4d unimpl:%-3d exit:%-3d output:%-3d fatal:%-3d framework:%-3d other:%-3d\n" \
    "TOTAL" "$total_pass" "$total_unimpl" "$total_exit" "$total_output" "$total_fatal" "$total_framework" "$total_other"

total_fail=$((total_unimpl + total_exit + total_output + total_fatal + total_framework + total_other))
echo ""
echo "Failure breakdown:"
echo "  unimplemented:  $total_unimpl  (missing command — skip-list candidate)"
echo "  exit-code:      $total_exit  (CLI shim returns wrong exit code)"
echo "  output-mismatch:$total_output  (CLI shim output format differs)"
echo "  fatal-error:    $total_fatal  (go-git library or CLI crash/error)"
echo "  test-framework: $total_framework  (git's test harness subtests)"
echo "  other:          $total_other  (uncategorized)"
echo ""
echo "To investigate potential go-git bugs, focus on 'fatal-error' failures."
echo "To improve CLI compatibility, focus on 'exit-code' and 'output-mismatch'."
