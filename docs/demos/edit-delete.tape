# Edit and delete your own messages. Rendered by docs/demos/record.sh.
Output docs/assets/demos/edit-delete.gif

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

Type "j"
Sleep 600ms
Type "{{JOIN_CODE_SILENT}}"
Enter
Sleep 2.5s

Type "meet at 9?" Enter
Sleep 800ms
Type "bring snacks" Enter
Sleep 1.5s

# Edit mode selects your latest message; move up to the first one.
Ctrl+E
Sleep 1s
Up
Sleep 1s
Enter
Sleep 800ms
Backspace 2
Type "10?"
Sleep 600ms
Enter
Sleep 2s

# Select the latest message again and delete it.
Ctrl+E
Sleep 800ms
Ctrl+E
Sleep 1s
Ctrl+D
Sleep 1.2s
Type "y"
Sleep 2.5s
