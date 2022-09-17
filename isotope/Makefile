SERVICE := service
CLIENT := tahler/fortio:prometheus
# NONE or ISTIO
ENVIRONMENT := NONE

# localhost:8080/fortio for tests

.PHONY: default
default: clean image image-minikube;

clean:
	rm -f *.{dot,svg,k8s.yaml} gen.yaml

image-no-cache:
	docker build --rm --no-cache -t $(SERVICE) -f service/Dockerfile .

image:
	docker build --rm -t $(SERVICE) -f service/Dockerfile .

image-minikube:
	docker build --rm -t $(SERVICE) -f service/Dockerfile .
	docker save $(SERVICE) > $(SERVICE).tar
	minikube image load $(SERVICE).tar
	rm $(SERVICE).tar

delete:
	kubectl delete --ignore-not-found=true deployment client
	kubectl delete --ignore-not-found=true ns service-graph

canonical:
	go run convert/main.go kubernetes example-topologies/canonical.yaml --service-image library/$(SERVICE) --client-image $(CLIENT) --environment-name=$(ENVIRONMENT) > canonical.k8s.yaml
	kubectl apply -f canonical.k8s.yaml

canonical-viz:
	go run convert/main.go graphviz example-topologies/canonical.yaml canonical.dot
	dot -Tsvg canonical.dot > canonical.svg

star:
	.venv/bin/python3 create_realistic_topology.py --type=star
	go run convert/main.go kubernetes gen.yaml --service-image $(SERVICE) --client-image $(CLIENT) --environment-name=$(ENVIRONMENT) > star.k8s.yaml
	kubectl apply -f star.k8s.yaml

star-viz:
	go run convert/main.go graphviz gen.yaml star.dot
	dot -Tsvg star.dot > star.svg

star-auxiliary:
	.venv/bin/python3 create_realistic_topology.py --type=star-auxiliary
	go run convert/main.go kubernetes gen.yaml --service-image $(SERVICE) --client-image $(CLIENT) --environment-name=$(ENVIRONMENT) > star-aux.k8s.yaml
	kubectl apply -f star-aux.k8s.yaml

star-aux-viz:
	go run convert/main.go graphviz gen.yaml star-aux.dot
	dot -Tsvg star-aux.dot > star-aux.svg

multitier:
	.venv/bin/python3 create_realistic_topology.py --type=multitier
	go run convert/main.go kubernetes gen.yaml --service-image $(SERVICE) --client-image $(CLIENT) --environment-name=$(ENVIRONMENT) > multitier.k8s.yaml
	kubectl apply -f multitier.k8s.yaml

multitier-viz:
	go run convert/main.go graphviz gen.yaml multitier.dot
	dot -Tsvg multitier.dot > multitier.svg

auxiliary-services:
	.venv/bin/python3 create_realistic_topology.py --type=auxiliary-services
	go run convert/main.go kubernetes gen.yaml --service-image $(SERVICE) --client-image $(CLIENT) --environment-name=$(ENVIRONMENT) > auxiliary-services.k8s.yaml
	kubectl apply -f auxiliary-services.k8s.yaml

auxiliary-services-viz:
	go run convert/main.go graphviz gen.yaml auxiliary-services.dot
	dot -Tsvg auxiliary-services.dot > auxiliary-services.svg
