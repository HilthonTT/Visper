# Join a room with its code and chat. A scripted friend (examples/demo-bot)
# owns the room and answers. Rendered by docs/demos/record.sh.
Output docs/assets/demos/chat.gif

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
Type@120ms "{{JOIN_CODE}}"
Sleep 600ms
Enter
Sleep 5s

Type "is this room really private?"
Sleep 400ms
Enter
Sleep 3s

Type "and what happens to the chat history?"
Sleep 400ms
Enter
Sleep 3.5s

Type "perfect, love it"
Sleep 400ms
Enter
Sleep 3.5s
