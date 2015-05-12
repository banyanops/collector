#/usr/bin/env python

import subprocess
from subprocess import Popen, PIPE

def executeCmd(cmd, args, out=None, err=None):
    p = Popen([cmd, args], stdout=PIPE)
    for line in iter(p.stdout.readline, b''):
        if line.startswith(b'#'):
            continue
        name = line.decode().split(':')
        print (name[0])

def main():
    executeCmd("cat", "/etc/passwd") 

main()
