package installation

// Frozen initializer source vectors read from immutable Git objects.
// docs/adr/0147-installed-ci-migration-preview.md:63.
// 8f2c6ae27f47a1ddbffc76cc10bdceffd424f2c4
var ciGeneratedRecipe = map[string]string{
	"internal/ciworkflow/application.go":  "9091ded458543801a526b436bd0527db37c1164583a1c891d42421c7d2265f7f",
	"internal/ciworkflow/release.go":      "ea70be98b512efe40e729904a9766afb43ad60ad002be5f30fa7fc8efa682f47",
	"internal/ciworkflow/workflow.go":     "34d56cb8d8ef4c347ae78764fa21775a4a443feeb4e1746c8b0388010f95313e",
	"internal/initcmd/answers.go":         "8466ce2cf052e1d8cb3d1d7dc831081d3514b6127694219c529f98bb23bd20a9",
	"internal/initcmd/boundaries.go":      "28689f5ac6204a2ec00c307bf14ab7c58b25aeacfe744746633d961f7c845c14",
	"internal/initcmd/ci.go":              "fa69e43ef955a89d714ca7e4de582bbfa8cb7bf53afa6fc568368d3e346f00fd",
	"internal/initcmd/command.go":         "98a635cdced05dd4ba48a26d43b65d77dc8e0aad707883f055b9c742bc875e11",
	"internal/initcmd/configuration.go":   "d838d5a2beeb9677a46eb96d1588b61aac8ef05c3508cbdef5ae38652dbf0cfb",
	"internal/initcmd/files.go":           "8dc90c67d030702ad2000a0697f2899f6e78949e56c7e759d387d09b5fd17442",
	"internal/initcmd/ignore_evidence.go": "c0d39b5a15ed7b7aceb39f0f518d8540d891033bcd9ddcd4ff5ae89f67f76f88",
	"internal/initcmd/input.go":           "2e782978acbabe97d27f5c5a35de161d9708aa27c3abf21c9f3e3ed6a8f3f39a",
	"internal/initcmd/messages.go":        "f5a11e7ba20639d8b48376ef23bfa736f5200fea9dd1ec493a24c924341c9f20",
	"internal/initcmd/plan.go":            "3d0c762247da15932b686d609a05e4485c3cd9f72bb6ef249f2ee4f34d4be0ea",
	"internal/initcmd/selection.go":       "ce7432427d6ff1a15867f34a216b995cd5ed24f28d3cd787582d60c1712836f2",
	"internal/initcmd/workflow.go":        "3da195943729779be2daf395e255068b4bfbd607c9ac7635fcaaf14392432669",
	"internal/initinventory/inventory.go": "6c5c0afffd167f7ce13128945347986facf58faecd3eda5a952f527ec36a76b7",
}

// 6b8f08c820c347a9c9d12c4afd00b3f7e6aca547
var ciCopiedRecipe = map[string]string{
	"internal/initcmd/answers.go":         "8466ce2cf052e1d8cb3d1d7dc831081d3514b6127694219c529f98bb23bd20a9",
	"internal/initcmd/boundaries.go":      "28689f5ac6204a2ec00c307bf14ab7c58b25aeacfe744746633d961f7c845c14",
	"internal/initcmd/ci.go":              "fa69e43ef955a89d714ca7e4de582bbfa8cb7bf53afa6fc568368d3e346f00fd",
	"internal/initcmd/command.go":         "d15c4155e0b01f917d6b444fb592071d28a355b7ca5486505c413e259355bc5e",
	"internal/initcmd/configuration.go":   "d838d5a2beeb9677a46eb96d1588b61aac8ef05c3508cbdef5ae38652dbf0cfb",
	"internal/initcmd/files.go":           "8dc90c67d030702ad2000a0697f2899f6e78949e56c7e759d387d09b5fd17442",
	"internal/initcmd/ignore_evidence.go": "c0d39b5a15ed7b7aceb39f0f518d8540d891033bcd9ddcd4ff5ae89f67f76f88",
	"internal/initcmd/input.go":           "2e782978acbabe97d27f5c5a35de161d9708aa27c3abf21c9f3e3ed6a8f3f39a",
	"internal/initcmd/messages.go":        "f5a11e7ba20639d8b48376ef23bfa736f5200fea9dd1ec493a24c924341c9f20",
	"internal/initcmd/plan.go":            "0886a95b0a71366ad808b510f2b341df3042e4685e07746bad5535cce81726dc",
	"internal/initcmd/selection.go":       "c1ee0bcec3dc9bf682b8baa89506e4c0cadda7fe630c6212b96faa10ca2d7b58",
	"internal/initinventory/inventory.go": "6c5c0afffd167f7ce13128945347986facf58faecd3eda5a952f527ec36a76b7",
}

// The earlier copied initializer kept its inventory inside initcmd.
// docs/adr/0148-transactional-installed-ci-migration.md:391.
// 95486ecdd3868a319a0e6362ee99cafd8f154200
var ciInlineInventoryCopiedRecipe = map[string]string{
	"internal/initcmd/answers.go":         "0ae298b61ce6f5c41fac44c0631dd5d71c94192179a60c645c4973ed766d9f75",
	"internal/initcmd/boundaries.go":      "28689f5ac6204a2ec00c307bf14ab7c58b25aeacfe744746633d961f7c845c14",
	"internal/initcmd/command.go":         "afcc397f65fae97fdfe16f4eaea772e29070f0882ccb167ffad7becd3f1bad22",
	"internal/initcmd/configuration.go":   "d838d5a2beeb9677a46eb96d1588b61aac8ef05c3508cbdef5ae38652dbf0cfb",
	"internal/initcmd/files.go":           "ee7c8698f7ed08a8d574b034aa8607a77a2f278ce3c4507df6a724237cb8d320",
	"internal/initcmd/ignore_evidence.go": "c0d39b5a15ed7b7aceb39f0f518d8540d891033bcd9ddcd4ff5ae89f67f76f88",
	"internal/initcmd/input.go":           "2e782978acbabe97d27f5c5a35de161d9708aa27c3abf21c9f3e3ed6a8f3f39a",
	"internal/initcmd/messages.go":        "f5a11e7ba20639d8b48376ef23bfa736f5200fea9dd1ec493a24c924341c9f20",
	"internal/initcmd/plan.go":            "7ddd35c732aa07d7abd9b9b765613e7053959089630a4bd38f2aaf74d8b4424d",
	"internal/initcmd/selection.go":       "8d6882e40ddf084e58a1dc963f90f820455ccd10c23ad1cda47520bbbd35d62e",
}
