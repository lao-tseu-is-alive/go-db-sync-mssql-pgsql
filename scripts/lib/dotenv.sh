#!/usr/bin/env bash
# Safe dotenv parsing shared by repository-owned operator scripts.

dotenv_trim_space() {
    local value="$1"
    value="${value#"${value%%[!$' \t\r\n']*}"}"
    value="${value%"${value##*[!$' \t\r\n']}"}"
    printf '%s' "${value}"
}

dotenv_parse_double_quoted_value() {
    local input="$1"
    local line_number="$2"
    local output=""
    local character next_character
    local index

    for ((index = 0; index < ${#input}; index++)); do
        character="${input:index:1}"
        if [[ "${character}" != '\' ]]; then
            output+="${character}"
            continue
        fi
        index=$((index + 1))
        [[ "${index}" -lt "${#input}" ]] || die "parse dotenv line ${line_number}: trailing backslash in double-quoted value"
        next_character="${input:index:1}"
        case "${next_character}" in
            '\') output+='\' ;;
            '"') output+='"' ;;
            n) output+=$'\n' ;;
            r) output+=$'\r' ;;
            t) output+=$'\t' ;;
            *) die "parse dotenv line ${line_number}: unsupported double-quoted escape" ;;
        esac
    done
    printf '%s' "${output}"
}

dotenv_load_missing() {
    local path="$1"
    local explicit="$2"
    local line line_number=0 name raw_value value

    if [[ ! -e "${path}" ]]; then
        [[ "${explicit}" -eq 0 ]] && return 0
        die "dotenv file not found: ${path}"
    fi
    [[ -f "${path}" && -r "${path}" ]] || die "dotenv path is not a readable regular file: ${path}"

    while IFS= read -r line || [[ -n "${line}" ]]; do
        line_number=$((line_number + 1))
        line="$(dotenv_trim_space "${line}")"
        [[ -z "${line}" || "${line}" == \#* ]] && continue
        if [[ "${line}" == export$'\t'* || "${line}" == export\ * ]]; then
            line="$(dotenv_trim_space "${line#export}")"
        fi
        [[ "${line}" == *=* && "${line}" != =* ]] || die "parse dotenv line ${line_number}: expected NAME=VALUE"
        name="$(dotenv_trim_space "${line%%=*}")"
        [[ "${name}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || die "parse dotenv line ${line_number}: invalid variable name"
        [[ -v "${name}" ]] && continue

        raw_value="$(dotenv_trim_space "${line#*=}")"
        value="${raw_value}"
        if [[ "${raw_value}" == \'* ]]; then
            [[ "${#raw_value}" -ge 2 && "${raw_value: -1}" == "'" ]] || die "parse dotenv line ${line_number}: unterminated single-quoted value"
            value="${raw_value:1:${#raw_value}-2}"
        elif [[ "${raw_value}" == \"* ]]; then
            [[ "${#raw_value}" -ge 2 && "${raw_value: -1}" == '"' ]] || die "parse dotenv line ${line_number}: unterminated double-quoted value"
            value="$(dotenv_parse_double_quoted_value "${raw_value:1:${#raw_value}-2}" "${line_number}")"
        fi
        printf -v "${name}" '%s' "${value}"
        export "${name}"
    done <"${path}"
}
