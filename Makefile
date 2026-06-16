generate-crd:
	controller-gen crd paths="./api/..." output:crd:artifacts:config=deploy/crd
	cp deploy/crd/*.yaml cf-tunnel-operator/crds/

generate-deepcopy:
	controller-gen object:headerFile="" paths="./api/..."

build:
	go build ./...

run:
	KUBECONFIG=$(KUBECONFIG) go run main.go

deploy-crd:
	kubectl apply -f deploy/crd --kubeconfig $(KUBECONFIG)