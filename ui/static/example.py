#!/usr/bin/python3
import json
import sys

import DRMSONIC

action = DRMSONIC.parse_params(sys.argv, "action")
doh = DRMSONIC.parse_params(sys.argv, "doh")
bind = DRMSONIC.parse_params(sys.argv, "bind")
proxy = DRMSONIC.parse_params(sys.argv, "proxy")
challenge = DRMSONIC.parse_params(sys.argv, "challenge")

DRMSONIC.session(bind=bind, proxy=proxy)
if doh != "":
    DRMSONIC.dns(doh)

if challenge == "cert":
    challenge = "CAQ="


def do_action():
    if action == "channels":
        print(
            json.dumps(
                {
                    "Channels": [
                        {
                            "Name": "Demo Channel",
                            "Mode": "live",
                            "SessionManifest": True,
                            "ManifestScript": "id=demo1",
                            "CdmType": "widevine",
                            "UseCdm": True,
                            "Cdm": "id=demo1",
                            "Video": "best",
                            "OnDemand": True,
                            "SpeedUp": True,
                        }
                    ]
                },
                indent=2,
            )
        )
        return

    if action == "events":
        print(
            json.dumps(
                {
                    "Events": [
                        {
                            "Name": "Demo Event",
                            "Mode": "live",
                            "SessionManifest": True,
                            "ManifestScript": "id=demo1",
                            "CdmType": "widevine",
                            "UseCdm": True,
                            "Cdm": "id=demo1",
                            "Video": "best",
                            "Start": 0,
                            "End": 0,
                        }
                    ]
                },
                indent=2,
            )
        )
        return

    if action == "heartbeat":
        sys.exit(0)

    if action == "manifest":
        print(
            json.dumps(
                {
                    "ManifestUrl": "",
                    "Headers": {
                        "Manifest": {
                            "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"
                        },
                        "Media": {
                            "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/107.0.0.0 Safari/537.36"
                        },
                    },
                    "Heartbeat": {"Url": "", "Params": "", "PeriodMs": 5 * 60 * 1000},
                }
            )
        )
        return

    if action == "cdm":
        print("00000000000000000000000000000000:00000000000000000000000000000000")
        return

    print("invalid action: " + action, file=sys.stderr)
    sys.exit(1)


do_action()
