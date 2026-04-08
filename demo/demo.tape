# From repo root (PATH must include vhs, ffmpeg, ttyd):
#   vhs demo/demo.tape
Output assets/demo.gif

Require go

Set Shell zsh
Set FontSize 15
Set Width 1120
Set Height 640
Set Padding 36
Set Margin 28
Set MarginFill "#000000"
Set BorderRadius 10
Set WindowBar Colorful
Set Theme {"name":"Aardvark Blue","black":"#191919","red":"#aa342e","green":"#4b8c0f","yellow":"#dbba00","blue":"#1370d3","magenta":"#c43ac3","cyan":"#008eb0","white":"#bebebe","brightBlack":"#454545","brightRed":"#f05b50","brightGreen":"#95dc55","brightYellow":"#ffe763","brightBlue":"#60a4ec","brightMagenta":"#e26be2","brightCyan":"#60b6cb","brightWhite":"#f7f7f7","background":"#000000","foreground":"#dddddd","cursor":"#007acc","selection":"#bfdbfe"}
Set TypingSpeed 70ms
Set PlaybackSpeed 1.1
Set CursorBlink false

Hide
Type "brew install --HEAD ./Formula/billy.rb" Sleep 200ms Enter
Sleep 10s
Show

Type "clear" Sleep 150ms Enter
Sleep 400ms
Type "billy --version" Sleep 200ms Enter
Sleep 2s

Type "clear" Sleep 150ms Enter
Sleep 400ms
Type "billy --help" Sleep 200ms Enter
Sleep 4s

Type "clear" Sleep 150ms Enter
Sleep 400ms
Type "billy analyze --provider anthropic testdata/anthropic/min.jsonl" Sleep 200ms Enter
Sleep 4s

Type "clear" Sleep 150ms Enter
Sleep 400ms
Type "billy forecast --provider anthropic --weekly-limit-usd 20 testdata/anthropic/min.jsonl" Sleep 200ms Enter
Sleep 4s
