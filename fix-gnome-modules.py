#!/usr/bin/env python3

import sys

filename = sys.argv[1]

with open(filename, "r") as inputfile:
    data = inputfile.read()

if filename.endswith(".desktop"):
    data = data.replace("Exec=/usr", "Exec=/snap/ubuntu-desktop-session/current/usr")
else:
    data = data.replace("'/usr", "'/snap/ubuntu-desktop-session/current/usr")

with open(filename, "w") as outputfile:
    outputfile.write(data)
