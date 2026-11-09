#!/bin/sh
exec ./bin/drmsonic -p 7779 -b 0.0.0.0 -providers providers -c drmsonic.json "$@"
