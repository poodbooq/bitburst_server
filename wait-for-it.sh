#!/bin/ash

set -e

while [[ $# -gt 0 ]]
do
  case "$1" in
    *:* )
      HOST=$(printf "%s\n" "$1"| cut -d : -f 1)
      PORT=$(printf "%s\n" "$1"| cut -d : -f 2)
      shift

      until nc -z $HOST $PORT; do
        >&2 echo "Waiting for $HOST:$PORT"
        sleep 1
      done
    ;;
    --)
      shift
      break
    ;;
  esac
done;

while [[ $# -gt 0 ]]
do
  $($1) # exec rest command
  shift
done;
