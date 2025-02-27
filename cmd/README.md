cmd Notes:

File Watching Basics: (fs-notify)
1. Detect when files change
2. determine what changed
3. trigger appropriate actions in response

1. Debouncing & Throttling - 
(editors usually: create temp files, write content, rename it to replace the og file) 
-> make sure this is detected as ONE big chance, rather than 3 separate actions.
Typically you timeout your actions to let things process.

2. Recursive watching ->
usually only detects for one folder. and some APIs may have limited support
new directory detection & consdieration.

3. Symlink/Network drives
- determine if following symlink/just the link itself for changes
- latency issues.