package catalog

import _ "embed"

// snapshot is a filtered models.dev catalog (relevant providers only), embedded
// so the app has an up-to-date model list without a network call. Regenerate
// with: curl -fsSL https://models.dev/api.json | jq '{openai,anthropic,...}'
//
//go:embed models_dev_snapshot.json
var snapshot []byte
