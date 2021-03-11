#!/bin/bash

chisel client \

# ⬇️ Authenticates user "foo" with password "bar"
--auth="foo:bar" \

# ⬇️ Connects to chisel relay server example.com
# listening on the default ("fallback") port, 8080
example.com \

# ⬇️ Reverse tunnels port 80 on the relay server to
# port 80 on your localhost. So you can expose
# that web server running on your laptop to the
# internet with a VPS.
R:80:localhost:80