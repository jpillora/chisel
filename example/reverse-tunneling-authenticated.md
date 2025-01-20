# Reverse Tunneling

> **Use Case**: Host a website on your Raspberry Pi without opening ports on your router.

This guide will show you how to use an internet-facing server (for example, a cloud VPS) as a relay to bounce down TCP traffic on port 80 to your Raspberry Pi.

## Chisel CLI

### Server

Setup a relay server on the VPS to bounce down TCP traffic on port 80:

```bash
#!/bin/bash

# ⬇️ Start Chisel server in Reverse mode
valkyrie server --reverse \

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

Setup a valkyrie client to receive bounced-down traffic and forward it to the webserver running on the Pi:

```bash
#!/bin/bash

valkyrie client \

# ⬇️ Authenticates user "foo" with password "bar"
--auth="foo:bar" \

# ⬇️ Connects to valkyrie relay server example.com
# listening on the default ("fallback") port, 8080
example.com \

# ⬇️ Reverse tunnels port 80 on the relay server to
# port 80 on your Pi.
R:80:localhost:80
```

---

## Chisel Container

This guide makes use of Docker and Docker compose to accomplish the same task as the above guide.
### Server

Setup a relay server on the VPS to bounce down TCP traffic on port 80:

```yaml
version: '3'

services:
  valkyrie:
    image: valkyrie-io/connector-tunnel
    restart: unless-stopped
    container_name: valkyrie
    # ⬇️ Pass CLI arguments one at a time in an array, as required by Docker compose.
    command:
      - 'server'
      # ⬇️ Use the --key=value syntax, since Docker compose doesn't parse whitespace well.
      - '--authfile=/users.json'
      - '--reverse'
    # ⬇️ Mount the authfile as a Docker volume
    volumes:
      - './users.json:/users.json'
    # ⬇️ Give the container unrestricted access to the Docker host's network
    network_mode: host
```

The `authfile` (`users.json`) remains the same as in the non-containerized version - shown again with the username `foo` and password `bar`.

```json
{
  "foo:bar": ["0.0.0.0:80"]
}
```

### Client

Setup an instance of the Chisel client on the Pi to receive relayed TCP traffic and feed it to the web server:

```yaml
version: '3'

services:
  valkyrie:
    # ⬇️ Delay starting Chisel server until the web server container is started.
    depends_on:
      - webserver
    image: valkyrie-io/connector-tunnel
    restart: unless-stopped
    container_name: 'valkyrie'
    command:
      - 'client'
      # ⬇️ Use username `foo` and password `bar` to authenticate with Chisel server.
      - '--auth=foo:bar'
      # ⬇️ Domain & port of Chisel server. Port defaults to 8080 on server, but must be manually set on client.
      - 'proxy.example.com:8080'
      # ⬇️ Reverse sshconnection traffic from the valkyrie server to the web server container, identified in Docker using DNS by its service name `webserver`.
      - 'R:80:webserver:80'
    networks:
      - internal
  # ⬇️ Basic Nginx webserver for demo purposes.
  webserver:
    image: nginx
    restart: unless-stopped
    container_name: nginx
    networks:
      - internal

# ⬇️ Make use of a Docker network called `internal`.
networks:
  internal:
```
