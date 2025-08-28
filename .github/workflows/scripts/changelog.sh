#!/bin/bash

# Find all directories containing a CHANGELOG.md file
changelogDirs=$(find . -type f -name 'CHANGELOG.md' -exec dirname {} \; | sort -u)
changedDirs=""

# Function to print a separator
print_separator() {
    echo "----------------------------------------"
}

# Check each directory for changes and ensure CHANGELOG.md is updated
for dir in $changelogDirs; do
    if [[ "$dir" == "." ]]; then
        continue
    fi
    print_separator
    echo "Checking: $dir"

    # Check if there are any changes in the directory
    dirChanges=$(git --no-pager diff -w --numstat origin/main -- $dir | wc -l)
    if [[ "$dirChanges" -gt 0 ]]; then
        echo " - changes detected"

        # Check if CHANGELOG.md has been modified
        changelogMod=$(git --no-pager diff -w --numstat origin/main -- $dir/CHANGELOG.md)
        if [[ -z "$changelogMod" ]]; then
            echo " - $dir/CHANGELOG.md not modified - Please update it with your changes before merging to main."
            exit 1
        else
            echo " - $dir/CHANGELOG.md modified"
            changelogLines=$(echo "$changelogMod" | awk '{print $1}')
            if [[ "$changelogLines" -lt 1 ]]; then
                echo " - didn't detect any substantial changes to CHANGELOG.md in $dir."
                exit 1
            else
                echo " - detected '$changelogLines' new non-whitespace lines in CHANGELOG.md in $dir. Thanks +1"
                changedDirs+=" $dir"
            fi
        fi
    else
        echo " - no changes detected"
    fi
done

print_separator

# Build exclusion pattern for directories with their own CHANGELOG.md
excludePatterns=(":!*/CHANGELOG.md")   # always exclude sub-CHANGELOG.md
for dir in $changedDirs; do
    excludePatterns+=(":!$dir/*")
done

echo "Checking root: ./"
# Check for changes in the root directory and subdirectories without their own CHANGELOG.md
rootChanges=$(git --no-pager diff -w --numstat origin/main -- . "${excludePatterns[@]}" | wc -l)
if [[ "$rootChanges" -gt 0 ]]; then
    echo " - changes detected"

    # Check if root CHANGELOG.md exists
    if [[ ! -f "./CHANGELOG.md" ]]; then
        echo "::warning:: - changes detected but no ./CHANGELOG.md was found"
    else
        echo " - ./CHANGELOG.md exists"
        rootChangelogMod=$(git --no-pager diff -w --numstat origin/main -- ./CHANGELOG.md)
        if [[ -z "$rootChangelogMod" ]]; then
            echo " - ./CHANGELOG.md not modified - Please update it with your changes before merging to main."
            exit 1
        else
            echo " - ./CHANGELOG.md modified"
            rootChangelogLines=$(echo "$rootChangelogMod" | awk '{print $1}')
            if [[ "$rootChangelogLines" -lt 1 ]]; then
                echo " - didn't detect any substantial changes to CHANGELOG.md in root CHANGELOG.md."
                exit 1
            else
                echo " - detected '$rootChangelogLines' new non-whitespace lines in root CHANGELOG.md. Thanks +1"
            fi
        fi
    fi
else
    echo " - no changes detected"
fi

print_separator
echo "All directories with changes have updated CHANGELOG.md files."
exit 0
