import os

import jinja2
import yaml

TEMPLATES_DIR = os.path.join(os.path.dirname(__file__), "..", "templates")


def load_values(values_path):
    with open(values_path) as f:
        return yaml.safe_load(f) or {"backends": []}


def render(template_name, context):
    env = jinja2.Environment(
        loader=jinja2.FileSystemLoader(TEMPLATES_DIR),
        trim_blocks=True,
        lstrip_blocks=True,
    )
    return env.get_template(template_name).render(context)
