import jinja2
import yaml


def load_values(values_path):
    with open(values_path) as f:
        return yaml.safe_load(f) or {"backends": []}


def render(template_name, context):
    # PackageLoader reads templates/ bundled inside the installed package
    # (see [tool.setuptools.package-data] in pyproject.toml) rather than a
    # path relative to this file, so rendering works from a wheel installed
    # anywhere, not just an editable checkout of this repo.
    env = jinja2.Environment(
        loader=jinja2.PackageLoader("gatewayctl", "templates"),
        trim_blocks=True,
        lstrip_blocks=True,
    )
    return env.get_template(template_name).render(context)
