#!/bin/bash

# Find all directories containing a CHANGELOG.md file
changelogDirs=$(find . -type f -name 'CHANGELOG.md' -exec dirname {} \;)

# Check each directory for changes and ensure CHANGELOG.md is updated
for dir in $changelogDirs; do
    echo "Checking directory: $dir"

    # Check if there are any changes in the directory
    dirChanges=$(git --no-pager diff -w --numstat origin/main -- $dir | wc -l)
    if [[ "$dirChanges" -gt 0 ]]; then
        echo "Changes detected in $dir"

        # Check if CHANGELOG.md has been modified
        changelogMod=$(git --no-pager diff -w --numstat origin/main -- $dir/CHANGELOG.md)
        if [[ -z "$changelogMod" ]]; then
            echo "CHANGELOG.md in $dir has not been modified. Please update it with your changes before merging to main."
            exit 1
        else
            echo "CHANGELOG.md in $dir has been modified. Verifying at least 1 (non-whitespace) line has been added."
            changelogLines=$(echo "$changelogMod" | awk '{print $1}')
            if [[ "$changelogLines" -lt 1 ]]; then
                echo "Didn't detect any substantial changes to CHANGELOG.md in $dir."
                exit 1
            else
                echo "Detected '$changelogLines' new non-whitespace lines in CHANGELOG.md in $dir. Thanks +1"
            fi
        fi
    else
        echo "No changes detected in $dir"
    fi
done

echo "All directories with changes have updated CHANGELOG.md files."
exit 0