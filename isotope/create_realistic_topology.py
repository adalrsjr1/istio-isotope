#!/usr/bin/env python3

# Copyright Adalberto Sampaio Junior @adalrsjr1
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import io
import random

from enum import Enum
from typing import Any
from igraph import Graph
from typing import Any, List

import yaml


def barabasi(n, m, power, attractiveness, directed):
    graph: Graph = Graph.Barabasi(
        n=n, m=m, power=power, zero_appeal=attractiveness, directed=directed)
    return graph


def direction(graph: Graph, directed: bool) -> Graph:
    # workaround to return undirected graph while preserves the properties
    # shown in Podolskiy, Vladimir, et al. "The weakest link: revealing
    # and modeling the architectural patterns of microservice applications."
    #  (2020): 113-122.

    if directed:
        # workaround to reverse the edges direction making id=0 the source not
        # the final target
        # https://igraph.discourse.group/t/create-tranpose-edge-reversed-of-graph/1219
        edgelist = graph.get_edgelist()
        transposed = (tuple(reversed(e)) for e in edgelist)
        return Graph(transposed, directed=True)
    return graph.as_undirected()


def weight_edges(graph: Graph) -> Graph:
    graph.es['weight'] = [random.random() for e in graph.es]
    return graph


def star(nodes: int, directed: bool) -> Graph:
    graph = barabasi(nodes, m=1, power=0.9, attractiveness=0.01,
                     directed=True)
    return direction(graph, directed)


def multitier(nodes: int, directed: bool) -> Graph:
    graph = barabasi(nodes, m=1, power=0.9, attractiveness=3.25,
                     directed=True)
    return direction(graph, directed)


def auxiliary_services(nodes: int, directed: bool) -> Graph:
    graph = barabasi(nodes, m=1, power=0.05,
                     attractiveness=3.25, directed=True)
    return direction(graph, directed)


def star_auxiliary(nodes: int, directed: bool) -> Graph:
    graph = barabasi(nodes, m=1, power=0.05,
                     attractiveness=0.01, directed=True)
    return direction(graph, directed)


class GraphModel(str, Enum):
    STAR = 'star'
    MULTITIER = 'multitier'
    AUXILIARY_SERVICES = 'auxiliary-services'
    STAR_AUXILIARY = 'star-auxiliary'


def sample(nodes: int, model: GraphModel, directed: bool, weighted: bool) -> Graph:
    models = {
        GraphModel.STAR: star(nodes, directed),
        GraphModel.MULTITIER: multitier(nodes, directed),
        GraphModel.AUXILIARY_SERVICES: auxiliary_services(nodes, directed),
        GraphModel.STAR_AUXILIARY: star_auxiliary(nodes, directed)
    }
    try:
        if weighted:
            return weight_edges(models[model])
        return models[model]
    except KeyError:
        raise ValueError(
            f'there is no graph model named as {model} try either: {list(models.keys())}')


class ModelFormat(str, Enum):
    NO_FORMAT = 'no-format'
    SVG = 'svg'
    CSV = 'csv'
    PLAIN_TEXT = 'plain-text'


def svg(graph: Graph) -> str:
    # TODO: library doesn't render edges weight

    data = io.StringIO()
    graph.write_svg(data)
    svg_data = data.getvalue()
    return svg_data


def csv(graph: Graph) -> str:
    class FakeIO:
        """
        Wraper for io.StringIO to provend Graph.write_dependency() to close
        the buffer right after it finish to writes the data internally
        """

        def __init__(self, stringIO: io.StringIO):
            self.stringIO: io.StringIO = stringIO

        def write(self, *args, **kargs):
            self.stringIO.write(*args, **kargs)

        def close(self, *args, **kargs):
            pass

    data = io.StringIO()
    mock_data = FakeIO(data)
    print_weight = {'attribute': 'weight'} if graph.is_weighted() else {}
    graph.write_adjacency(f=mock_data, sep=',', eol='\n', **print_weight)
    csv_data = data.getvalue()
    data.close()

    return csv_data


def adjacency(graph: Graph) -> Any:
    if graph.is_weighted():
        return graph.get_adjacency(attribute='weight')
    return graph.get_adjacency()

#####
# Creates a scale free graph according to:
#
#  - [1] [Improving Microservice-based Applications with Runtime Adaptation](https://doi.org/10.1186/s13174-019-0104-0)
#  - [2] [The Weakest Link: Revealing and Modeling the Architectural Patterns of Microservice Applications](https://doi.org/10.5555/3432601.3432616)
#  - [3] [Graph-Based Analysis and Prediction for Software Evolution](https://doi.org/10.1109/ICSE.2012.6227173)
#  - [4] [Power Laws in Software](https://doi.org/10.1145/1391984.1391986)
#####


REQUEST_SIZE = 128
RESPONSE_SIZE = 128
NUM_REPLICAS = 1

# Depth of the tree.
TYPE_GRAPH = GraphModel.MULTITIER
NUM_SERVICES = 10


def parse_arguments():
    import argparse
    global REQUEST_SIZE
    global RESPONSE_SIZE
    global NUM_REPLICAS
    global NUM_SERVICES
    global TYPE_GRAPH

    parser = argparse.ArgumentParser(description='Process some integers.')
    parser.add_argument('--req-size', metavar='q', type=int,
                        default=REQUEST_SIZE,
                        help='an integer for the accumulator')

    parser.add_argument('--res-size', metavar='s', type=int,
                        default=RESPONSE_SIZE,
                        help='an integer for the accumulator')

    parser.add_argument('--replicas', metavar='r', type=int,
                        default=NUM_REPLICAS,
                        help='an integer for the accumulator')

    parser.add_argument('--num-services', metavar='n', type=int,
                        default=NUM_SERVICES,
                        help='an integer for the accumulator')

    parser.add_argument('--type', metavar='t', type=GraphModel, required=True,
                        help='graph model: star, star_auxiliary, multitier, auxiliary-services')

    args = parser.parse_args()

    REQUEST_SIZE = args.req_size
    RESPONSE_SIZE = args.res_size
    NUM_REPLICAS = args.replicas
    NUM_SERVICES = args.num_services
    TYPE_GRAPH = args.type


def main() -> None:

    g: Graph = sample(NUM_SERVICES, TYPE_GRAPH, directed=True, weighted=False)

    class Service:
        def __init__(self, name, children, entrypoint: bool = False):
            self.isEntrypoint: bool = entrypoint
            self.name: str = 'mock-'+str(name)
            self.script: List[str] = ['mock-'+str(child) for child in children]

        def marshal(self) -> dict:
            result = {
                'name': self.name,
                'script': [{'call': call} for call in self.script]
            }

            if self.isEntrypoint:
                result['isEntrypoint'] = self.isEntrypoint

            return result

    adjlist = g.get_adjlist()
    services = []
    for i, svc in enumerate(adjlist):
        services.append(Service(i, svc, i == 0).marshal())

    with open('gen.yaml', 'w') as f:
        yaml.dump(
            {
                'defaults': {
                    'requestSize': REQUEST_SIZE,
                    'responseSize': RESPONSE_SIZE,
                    'numReplicas': NUM_REPLICAS,
                },
                'services': services,
            },
            f,
            default_flow_style=False)


if __name__ == '__main__':
    parse_arguments()
    main()
