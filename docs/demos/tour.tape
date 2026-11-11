# Menu palette, FAQ, settings and help. Rendered by docs/demos/record.sh.
Output docs/assets/demos/tour.gif

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

# The menu palette (m): arrow down to the FAQ and open it.
Sleep 1.2s
Type "m"
Sleep 1.5s
Down
Sleep 500ms
Down
Sleep 700ms
Enter
Sleep 1.5s
# The FAQ is an accordion: up/down open the next answer.
Down
Sleep 1.4s
Down
Sleep 1.6s
# Left/right (or tab) move between the tabs at the top.
Right
Sleep 1.5s
# Settings: pick the chat companion; the preview updates live.
Type "2"
Sleep 2.2s
Type "1"
Sleep 2s
# ? shows every shortcut.
Type "?"
Sleep 3s
Escape
Sleep 1.2s
