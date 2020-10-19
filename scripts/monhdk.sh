#!/bin/bash

HOST=$1

/usr/bin/curl -s "http://$HOST/debug/vars" | /usr/bin/jq -r '[
	(.cmdline|join(" ")),
	(.image|tostring),
	(.science|tostring),
	(.total|tostring),
	(.size|tostring),
	(.errors|tostring),
	(.skipped|tostring),
	(.uptime|tostring),
	(.wait|tostring)
] | join("\n")'
