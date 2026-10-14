#!/usr/bin/env bash
set -e

# Перейти в папку скрипта, откуда бы его ни запустили.
cd "$(dirname "$0")"

# Создать Python-окружение один раз и активировать его.
if [ ! -d .venv ]; then
  python3 -m venv .venv
fi
source .venv/bin/activate

# Установить зависимости Ansible в папку проекта.
python -m pip install -r requirements-lock.txt
ansible-galaxy collection install -r requirements.yml -p .collections

# Передать playbook все аргументы скрипта, например --tags preflight.
ansible-playbook playbook.yml "$@"
