package installation

// Closed review source vector from immutable merged 3794e6cc8fa66eb02504e753b0ad11955f5cf85a.
// It is also identical in the original packaged 95486ecdd3868a319a0e6362ee99cafd8f154200 family.
// docs/adr/0154-installed-review-workflow-migration.md:59.
var ciReviewOriginalRecipe = map[string]string{
	"internal/reviewlanecmd/command.go":  "985df27b0f9395e67c28fe7f8683c3e463690a29b8f31157f22a07e12386f42e",
	"internal/reviewlanecmd/files.go":    "f0bc7e783547ac0f38a785e5bd28c5263f1a68f697ae735a028011b28df82a58",
	"internal/reviewlanecmd/mutation.go": "5dbc274ed5e01b7c0aa555699711df6521a6313c7892806aa17d846d378bbc6f",
	"internal/reviewlanecmd/settings.go": "2068b10cc171778a22f1a5cbcc46c57c7ae9bc08ea16737a825c6e52b31d540e",
}

// Decision 170 extracts the same literal projection without changing enable policy.
// Replay qualifies the original source bytes; it never invokes this package's
// current renderer to reconstruct an older transaction.
// docs/adr/0154-installed-review-workflow-migration.md:78.
var ciReviewSharedRecipe = map[string]string{
	"internal/reviewlanecmd/command.go":  "985df27b0f9395e67c28fe7f8683c3e463690a29b8f31157f22a07e12386f42e",
	"internal/reviewlanecmd/files.go":    "f0bc7e783547ac0f38a785e5bd28c5263f1a68f697ae735a028011b28df82a58",
	"internal/reviewlanecmd/mutation.go": "528072f46775d8964a577a5ea26d4136c64b9e5a61183f10dac4be477c02c48e",
	"internal/reviewlanecmd/render.go":   "e44d20e704296cec7cb4a4b4ed9b3306e8a161ccc0f2a44eac83103e43783a7d",
	"internal/reviewlanecmd/settings.go": "2068b10cc171778a22f1a5cbcc46c57c7ae9bc08ea16737a825c6e52b31d540e",
}
