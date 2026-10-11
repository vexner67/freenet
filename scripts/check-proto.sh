#!/usr/bin/env bash
# Checks that the generated protobuf code of every service is what its
# .proto gives: each is generated anew into a temporary directory, with the
# flags of the services' `make proto`, and compared with the committed
# *.pb.go. A .proto changed without `make proto`, or code edited by hand,
# fails here.
#
# The generators write their versions into the code, so they must be the
# very ones the code was generated with. Changing them, change them here
# and in .github/workflows/ci.yml, and run `make proto` in every service.
set -euo pipefail

cd "$(dirname "$0")/.."

protoc_version=34.1
gen_go_version=v1.36.12
gen_go_grpc_version=1.6.2

die() {
	echo "check-proto: $*" >&2
	exit 1
}

# need checks that a tool is installed and of the expected version.
need() {
	local tool=$1 want=$2 install=$3 got

	command -v "$tool" >/dev/null || die "$tool is not installed: $install"
	got=$("$tool" --version | awk '{ print $NF }')
	[ "$got" = "$want" ] || die "$tool is $got, the code is generated with $want: $install"
}

need protoc "$protoc_version" "https://github.com/protocolbuffers/protobuf/releases/tag/v$protoc_version"
need protoc-gen-go "$gen_go_version" "go install google.golang.org/protobuf/cmd/protoc-gen-go@$gen_go_version"
need protoc-gen-go-grpc "$gen_go_grpc_version" "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v$gen_go_grpc_version"

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

stale=()
for proto in */api/*/v1/*.proto; do
	service=${proto%%/*}
	module=github.com/vexner67/freenet/$service
	mkdir -p "$out/$service"

	(cd "$service" && protoc \
		--proto_path=. \
		--go_out="$out/$service" \
		--go_opt="module=$module" \
		--go-grpc_out="$out/$service" \
		--go-grpc_opt="module=$module" \
		"${proto#*/}") || die "protoc failed for $proto"

	dir=$(dirname "${proto#*/}")
	# Only *.pb.go: handwritten Go may live next to it. Both ways, so
	# committed code that is no longer generated is found too.
	committed=$(cd "$service/$dir" && ls ./*.pb.go 2>/dev/null || true)
	generated=$(cd "$out/$service/$dir" && ls ./*.pb.go)
	if [ "$committed" != "$generated" ]; then
		stale+=("$service")
		continue
	fi

	for file in $generated; do
		if ! cmp -s "$service/$dir/$file" "$out/$service/$dir/$file"; then
			stale+=("$service")
			break
		fi
	done
done

if [ ${#stale[@]} -gt 0 ]; then
	die "generated code differs from the .proto, run \`make proto\` in: ${stale[*]}"
fi

echo "check-proto: generated code matches the .proto of every service"
