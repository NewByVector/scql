from mako.template import Template
from pathlib import Path
from dotenv import load_dotenv
import os
import subprocess
import sys
import base64
import json

CUR_PATH = Path(__file__).parent.resolve()
TEMPLATE_PATH = os.path.join(CUR_PATH, "docker-compose-tmpl")
PROTOCOLS_ENV_NAME = "PROTOCOLS"
MYSQL_PORT_ENV_NAME = "MYSQL_PORT"
SCDB_PORTS_ENV_NAME = "SCDB_PORTS"
SCQL_IMAGE_NAME_ENV_NAME = "SCQL_IMAGE_TAG"
POSTGRES_PORT_ENV_NAME = "POSTGRES_PORT"

DOCKER_COMPOSE_YAML_FILE = os.path.join(CUR_PATH, "docker-compose.yml")
MYSQL_TEMPLATE = os.path.join(TEMPLATE_PATH, "docker-compose.yaml")
DOCKER_COMPOSE_TEMPLATE = os.path.join(TEMPLATE_PATH, "docker-compose.template")
DATASOURCE_TEMPLATE = os.path.join(TEMPLATE_PATH, "datasource.template")
ENGINE_TEMPLATE = os.path.join(TEMPLATE_PATH, "engine.template")
SCDB_TEMPLATE = os.path.join(TEMPLATE_PATH, "scdb.template")
SCDB_TEMPLATE_PATH = os.path.join(CUR_PATH, "scdb/conf_tmpl")
SCDB_CONF_TEMPLATE = os.path.join(SCDB_TEMPLATE_PATH, "config.yml.template")

PARTY = ["alice", "bob", "carol"]


def split_string(str):
    splitted_str = str.split(",")
    trimmed_str = []
    for s in splitted_str:
        trimmed_str.append(s.lstrip().rstrip())
    return trimmed_str


def create_docker_compose_yaml():
    load_dotenv(override=True)
    protocols = split_string(os.getenv(PROTOCOLS_ENV_NAME))
    scdb_ports = split_string(os.getenv(SCDB_PORTS_ENV_NAME))
    assert len(protocols) == len(scdb_ports)
    mysql_port = os.getenv(MYSQL_PORT_ENV_NAME)
    pg_port = os.getenv(POSTGRES_PORT_ENV_NAME)
    image_tag = os.getenv(SCQL_IMAGE_NAME_ENV_NAME)
    scdb_template = Template(filename=SCDB_TEMPLATE)
    scdb_docker = ""
    for i, p in enumerate(protocols):
        print(p)
        scdb_docker += scdb_template.render(
            PROTOCOL=p, SCQL_IMAGE_TAG=image_tag, SCDB_PORT=scdb_ports[i]
        )

    engine_template = Template(filename=ENGINE_TEMPLATE)
    engine_docker = ""
    for p in PARTY:
        engine_docker += engine_template.render(PARTY=p, SCQL_IMAGE_TAG=image_tag)
    datasource_template = Template(filename=DATASOURCE_TEMPLATE)
    datasource_docker = datasource_template.render(
        MYSQL_PORT=mysql_port, POSTGRES_PORT=pg_port
    )

    docker_compose_template = Template(filename=DOCKER_COMPOSE_TEMPLATE)
    docker_compose = docker_compose_template.render(
        ENGINE=engine_docker, SCDB=scdb_docker, DATASOURCE=datasource_docker
    )
    with open(DOCKER_COMPOSE_YAML_FILE, "w") as f:
        f.write(docker_compose)

    # create scdb config
    conf_template = Template(filename=SCDB_CONF_TEMPLATE)
    for p in protocols:
        dst_path = os.path.join(CUR_PATH, f"scdb/conf/{p}")
        if not os.path.exists(dst_path):
            os.makedirs(dst_path)
        with open(os.path.join(dst_path, "config.yml"), "w") as f:
            f.write(conf_template.render(PROTOCOL=p))

def generate_private_keys():
    for p in PARTY:
        pem_path = os.path.join(CUR_PATH, f"engine/{p}/conf/ed25519key.pem")
        try:
            result = subprocess.run(["openssl", "genpkey", "-algorithm", "ed25519", "-out", pem_path])
            result.check_returncode()
        except subprocess.CalledProcessError as e:
            print(e, file=sys.stderr)


def generate_authorized_profiles():
    pubkeys = dict()
    for p in PARTY:
        pem_path = os.path.join(CUR_PATH, f"engine/{p}/conf/ed25519key.pem")
        result = subprocess.run(["openssl", "pkey", "-in", pem_path, "-pubout", "-outform", "DER"], capture_output=True)
        result.check_returncode()
        pubkey = base64.standard_b64encode(result.stdout).decode()
        pubkeys[p] = pubkey
    for p in PARTY:
        parties = list()
        peers = set(PARTY)
        peers.remove(p)
        for peer in peers:
            party = {
                "party_code": peer,
                "public_key": pubkeys[peer],        
            }
            parties.append(party)
        profile = dict()
        profile["parties"] = parties
        profile_path = os.path.join(CUR_PATH, f"engine/{p}/conf/authorized_profile.json")
        with open(profile_path, "w", encoding="utf-8") as f:
            json.dump(profile, f, indent=4, ensure_ascii=False)

        


if __name__ == "__main__":
    create_docker_compose_yaml()
    generate_private_keys()
    generate_authorized_profiles()
