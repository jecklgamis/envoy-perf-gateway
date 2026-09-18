default:
	@cat ./Makefile
venv:
	python3 -m venv .venv
	.venv/bin/pip install -e ".[s3,build]"
# Builds a wheel + sdist into dist/. Self-contained - installable anywhere
# with `pip install dist/*.whl`, not just editable from this repo.
build: venv
	.venv/bin/python3 -m build
install: build
	pip install --force-reinstall dist/*.whl
clean:
	rm -rf dist build *.egg-info .venv
