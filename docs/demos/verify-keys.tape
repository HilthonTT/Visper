# Join a room, then compare the owner's key fingerprint and verify it. A
# quiet member (examples/demo-bot) owns the room. Rendered by
# docs/demos/record.sh.
Output docs/assets/demos/verify-keys.gif

Set Shell bash
Set FontSize 16
Set Width 1280
Set Height 760
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set Framerate 24

Hide
Type "clear && visper" Enter
Sleep 3.5s
Show

Sleep 800ms
Type "j"
Sleep 800ms
Type@120ms "{{JOIN_CODE_SILENT}}"
Sleep 600ms
Enter
Sleep 4s

# Open the member list (the owner is highlighted first) and their key.
Ctrl+S
Sleep 1.5s
Ctrl+K
Sleep 4s

# The digits matched what they read out: verify.
Type "y"
Sleep 3s
Escape
Sleep 2.5s
