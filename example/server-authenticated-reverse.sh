#!/bin/bash

# ⬇️ Start Chisel server in Reverse mode
# i.e. to use a cloud VPS to expose
# a server running on your laptop to 
# the internet.
chisel server --reverse \

# ⬇️ Use the include users.json as an authfile
--authfile="./users.json" \