#!/bin/sh
set -e
exec ./bin/drmsonic -p 1234 -b 0.0.0.0 -providers providers -c drmsonic.json "$@"
