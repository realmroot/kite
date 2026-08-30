#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

for path in pkg/ai ui/src/components/ai-chat ui/src/contexts/ai-chat-context.tsx ui/src/hooks/use-ai-chat.ts; do
  if [ -e "$path" ]; then
    echo "embedded AI path still exists: $path" >&2
    exit 1
  fi
done

if rg -n -i \
  '(ai-chat|use-ai-chat|aiAgent|AIProvider|AIModel|AIAPI|AIMax|PendingSession|github\.com/(anthropics|openai)|built-in AI|AI assistant|AI 助手)' \
  --hidden \
  --glob '!**/.git/**' \
  --glob '!static/**' \
  --glob '!docs/architecture/product-decisions.md' \
  --glob '!scripts/verify-architecture.sh' \
  --glob '!scripts/verify-no-embedded-ai.sh' \
  .; then
  echo "embedded AI references remain" >&2
  exit 1
fi
