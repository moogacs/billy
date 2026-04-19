<!-- BILLY_RT_PROXY:START -->
# Billy Runtime Proxy

Use billy runtime compaction for common shell commands to reduce token usage.

Prefer:

- billy proxy -- git status
- billy proxy -- git diff
- billy proxy -- go test ./...
- billy proxy -- rg "pattern" .
- billy gain

Do not rewrite commands containing shell control operators.
<!-- BILLY_RT_PROXY:END -->


