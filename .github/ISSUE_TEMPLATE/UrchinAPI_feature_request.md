---
name: Pipe
about: The multi Prosses multiplexor needes it
title: "Pipes"
labels: Urchin API
assignees: Gabriel-Weaver

---

**Is your feature request related to a problem? Please describe.**
I need two independent prosses to communicate with each other over a pipe [...]

**Describe the solution you'd like**
Pipes please

**Describe alternatives you've considered**
Perhaps internal routing of TCP packets, but that makes it to complacted

**Additional context**
In the Api We will be dynamically opening new pipes to each API instance thus I will need a pipe to each and an open global pipe to ask for new connections, I understand that if a message on a pipe is small enough its atomic


---
name: Serial
about: this is how we communcate with the ESP32
title: "Serial"
labels: Urchin API
assignees: Gabriel-Weaver

---

**Is your feature request related to a problem? Please describe.**
I need Serial to comuncate with the EESP32
[...]

**Describe the solution you'd like**
Serial

**Describe alternatives you've considered**


**Additional context**



---
name: Threads
about: I need this as it allows the API to act indepently
title: "Threads"
labels: Urchin API
assignees: Gabriel-Weaver

---

**Is your feature request related to a problem? Please describe.**
I need Threads so the API can call fucntion outside of the main execution path
[...]

**Describe the solution you'd like**
Threads: everything we could do before

**Describe alternatives you've considered**
none

**Additional context**

