// Stub module boundary: fences the evolve-harness tree off from the parent
// torus_go_agent module so `go build/vet/test ./...` at the repo root skips it.
// The search_db*/harness_*/src directories hold partial source snapshots that
// do not compile in place (run.sh applies them over internal/ before building),
// and tasks/*/workspace dirs are standalone modules copied to tmpdirs by
// evaluate.py. Nothing in here is meant to build as part of the main module.
module evolve-harness-snapshots

go 1.25
