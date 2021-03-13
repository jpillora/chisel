# Reverse Tunneling

> **Use Case**: Host a website on your Raspberry Pi without opening ports on your router.

This guide will show you how to use an internet-facing server (for example, a cloud VPS) as a relay to bounce traffic down to your local webserver.

## Chisel CLI
### Server

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

### Client

Setup a chisel client to receive bounced-down traffic and forward it to the webserver running on the Pi:

```bash
#!/bin/bash

chisel client \

# ⬇️ Authenticates user "foo" with password "bar"
--auth="foo:bar" \

# ⬇️ Connects to chisel relay server example.com
# listening on the default ("fallback") port, 8080
example.com \

# ⬇️ Reverse tunnels port 80 on the relay server to
# port 80 on your Pi.
R:80:localhost:80
```

---

## Chisel Docker Container

This guide makes use of Docker and docker-compose to accomplish the same task as the above guide, using the chisel container.

It assumes your webserver is also containerized and listening on port 80.

### Server

```yaml
version: '3'

services:
  chisel:
    image: jpillora/chisel
    restart: unless-stopped
    container_name: chisel
    # ⬇️ Pass CLI arguments one at a time in an array, as required by compose.
    command:
      - 'server'
      - '--authfile=/users.json'
      - '--reverse'
    # ⬇️ Mount the authfile as a docker volume
    volumes:
      - './users.json:/users.json'
    # ⬇️ Give the container unrestricted access to the docker host's network
    network_mode: host
```