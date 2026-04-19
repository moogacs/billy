<!-- BILLY_RT_PROXY:START -->
# Billy Runtime Proxy

Use billy runtime compaction for shell-heavy commands to reduce token usage.

Prefer:

- billy proxy -- git status
- billy proxy -- git diff
- billy proxy -- go test ./...
- billy proxy -- rg "pattern" .
- billy gain

When a shell command includes pipelines/control operators, run it normally.
<!-- BILLY_RT_PROXY:END -->


