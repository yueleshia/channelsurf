#!/bin/sh

DEBUG="0" # 0 regular, 1 do zero and save, 2 use local dl

main() {
  #query_live_metadata limelicious
  #query_live_metadata valkyrae

  channel="${2}"
  offset="${3}"

  case "${1}"
  in f|follow)
    for channel in $( cat cannel_list.txt ); do
      view_channel "${channel}" true
    done
  ;; v|vods) list_vods "${channel}"
  ;; o|open) view_channel "${channel}" false "${offset}"
  ;; *)
    <<EOF cat -
USAGE: (Use first character or full word)
  ${0##*/} follow                    - list online status of various channels
  ${0##*/} open <channel> [<offset>] - see latest vods
  ${0##*/} vods <channel> [<offset>] - see latest vods
EOF
  esac
}

list_vods() {
  id="${1}"

  [ "" = "${id}" ] && { printf %s\\n "Please specify a channel" >&2; exit 1; }

  x="$( read_twitch "${id}/videos?filter=archives&sort=time" )" || exit "$?"
  if [ "" = "${x}" ]; then
    printf %s\\n "${x}" >&2
    exit 1
  fi
  x="$( printf %s\\n "${x}" | jq --raw-output '.["@graph"]
    | map(select(.["@type"] == "ItemList"))[0]
    | .itemListElement
    # Filter out clips, etc.
    | map(select(.url | startswith("https://www.twitch.tv/videos/")))
    | sort_by(.uploadDate)
    | reverse
    | map("* \(.name)\u001E\(.description)\u001E\(.uploadDate)\u001E\(.duration)\u001E\(.url)")
    | join("\u0000")
  ' | fzf --read0 )" || exit "$?"
  x="$( printf %s\\n "${x}" | awk -v FS="$( printf '\x1E' )" '{ print $5 }' )"
  printf %s\\n "${x}" | clipboard.sh -w
  watch "${x}"
}

watch() {
  streamlink --hls-start-offset "${2:-0}" --twitch-disable-ads "${1}"
}

view_channel() {
  id="${1}"
  is_dry="${2}"
  offset="${3}"

  [ "" = "${id}" ] && { printf %s\\n "Please specify a channel" >&2; exit 1; }

  x="$( read_twitch "${1}" )" || exit "$?"
  x="${x:-"{}"}"
  metadata="$( printf %s\\n "${x}" | jq '.["@graph"][0]' )"
  if [ "null" = "${metadata}" ]; then
    printf %s\\n "'${id}' is offline" >&2
    [ false = "${is_dry}" ] && watch "https://www.twitch.tv/${id}" "${offset}"

  else
    printf %s "'${id}' is live." >&2
    printf %s\\n "${metadata}" | jq --raw-output '.description'
    [ false = "${is_dry}" ] && watch "https://www.twitch.tv/${id}" "${offset}"
  fi
}

# Twitch uses React I believe
# Most modern UI frameworks will stream represent the UI as a data blob, which
# is then hydrated and rendered to the DOM. We are reading this data blob.
read_twitch() {
  endpoint="${1}"

  x="$( fancy_curl "https://www.twitch.tv/${endpoint}" )" || { printf %s\\n "${x}" >&2; exit "$?"; }
  printf %s\\n "Parsing html" >&2
  x="$( printf %s\\n "${x}" | pup 'head script[type="application/ld+json"] text{}' )"
  printf %s\\n "${x}"
}

fancy_curl() (
  url="${1}"

  [ "" = "${url}" ] && { printf %s\\n "Please provide a url"; exit 1; }

  case "${DEBUG}"
  in 0) html="$( curl --silent -i --location "${url}" )" || exit 1
  ;; 1) html="$( curl --silent -i --location "${url}" )" || exit 1
        printf %s\\n "${html}" >test.html
  ;; 2) html="$( cat test.html )" || exit "$?"
  ;; *) printf %s\\n "Invalid DEBUG"; exit 1
  esac
  printf %s\\n "${html}" >orig.html

  for x in 0 1 2 3; do
    status="$( printf %s\\n "${html}" | sed -n '1s/ [A-Za-z ]*\r$//p' | sed -n 's/^.* //p' )"
    [ "" = "${status}" ] && break
    html="$( printf %s\\n "${html}" | awk '
      NR == 1 && /^HTTP/   { start = 1 }
      start == 1 && /^\r$/ { start = 0; next; }
      start == 0           { print $0  }
    ' )"
    if [ "${status}" -ge 400 ]; then
      printf %s\\n "Bad HTTP status code ${status}" >&2
      exit 1
    fi
  done

  printf %s\\n "${html}"
)

# @TODO: check if clientid is still valid
<&0 main "$@"
