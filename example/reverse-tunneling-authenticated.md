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

