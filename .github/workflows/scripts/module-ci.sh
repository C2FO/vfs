#!/bin/sh

function get_changed_modules() {
    # exit if $1 and $2 are not defined
    if [ -z "$1" ] || [ -z "$2" ]; then
        echo "Usage: get_changed_modules <modules> <originCommit>"
        exit 1
    fi

    local modules=$1
    local originCommit=$2
    local files=$(git diff --stat $originCommit --name-only | cat | awk -F"|" '{print "\""$1"\""}' | jq -s '.')
    local filtered='{}'

    # Add module as key to filtered jq object. Set the value to an empty array.
    for dir in $(echo "$modules" | jq -r '.[]'); do
        filtered=$(echo "$filtered" | jq --arg dir "${dir:2}" '. + {($dir): []}')
        for file in $(echo "$files" | jq -r '.[]'); do
            # If it's not the root directory, and the filename is contained in the module directory, include it in the
            # results.
            if [[ $(paths_share_root ${dir:2} $(dirname $file)) ]] && [[ "$(dirname $file)" != "." ]]; then
                # Add file to the array of files for the module
                # {
                #   "path/to/module": ["path/to/module/file1", "path/to/module/file2"]
                # }
                filtered=$(echo "$filtered" | jq --arg dir "$dir" --arg file "$file" '.[$dir] += [$file]')
            fi
        done
    done

    # create jq array for filtered keys that have an array length greater than 0
    filtered_keys="$(echo "$filtered" | jq '. | to_entries[] | select(.value | length > 0) | .key' | jq -s '.')"

    # join filtered_keys with root module
    echo "$filtered_keys" | jq -c -r
}

# Checks if two paths share a common root. Paths must use the same prefix pattern (i.e. path/to/dir vs. ./path/to/dir)
function paths_share_root() {
    local path1=$1
    local path2=$2
    # If the paths are the same, they share a root
    if [[ "$path1" == "$path2" ]]; then
        echo "true"
        return 1
    fi

    if [[ "$path1" == "$path2"* ]] || [[ "$path2" == "$path1"* ]]; then
        echo "true"
        return 1
    fi

    return 0
}
