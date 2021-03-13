# Reverse Tunneling

This guide will show you how to use a publicly-visible server to relay traffic. For example, to self-host a website on port 80 without opening ports on your router.

## Server

Setup a relay server to bounce down traffic on port 80:

```bash
#!/bin/bash

# ⬇️ Start Chisel server in Reverse mode
chisel server --reverse \

# ⬇️ Use the include users.json as an authfile
--authfile="./users.json" \
```

The corresponding `authfile` might look like this:

```json
{
  "foo:bar": ["0.0.0.0:80"]
}
```

## Client

Setup a chisel client to receive bounced-down traffic and forward it to the webserver running on `localhost:80`

```bash
#!/bin/bash

chisel client \

# ⬇️ Authenticates user "foo" with password "bar"
--auth="foo:bar" \

# ⬇️ Connects to chisel relay server example.com
# listening on the default ("fallback") port, 8080
example.com \

# ⬇️ Reverse tunnels port 80 on the relay server to
# port 80 on your localhost. So you can expose
# that web server running locally to the internet.
R:80:localhost:80
```
