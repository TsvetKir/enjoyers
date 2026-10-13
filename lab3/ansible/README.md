# Автоматизация лабораторной № 3

Все Ansible-файлы собраны в этой директории. Исходники приложения, Dockerfile,
политики и Helm chart находятся на уровень выше. Путь к ним задаётся в
`settings.yml` через `{{ playbook_dir | dirname }}`.

Запуск из каталога lab3:

```bash
bash ansible/run.sh
```

Только предварительная проверка без развёртывания:

```bash
bash ansible/run.sh --tags preflight
```

`run.sh` переходит в собственный каталог, активирует локальную `.venv`,
устанавливает зависимости и запускает playbook. Повторный запуск обновляет
лабораторную в существующем кластере.

- `playbook.yml` — последовательность этапов.
- `tasks/` — проверки, кластер, образы, чарты, развёртывание и проверка ресурсов.
- `settings.yml` — изменяемые параметры.
- `inventory.ini`, `ansible.cfg` — локальное подключение и настройки Ansible.
- `requirements*.txt`, `requirements.yml` — Python-зависимости и коллекция Kubernetes.
- `.state/` — kubeconfig и кэш чартов; не включается в Git.
- `.venv-previous/` — прежнее окружение, сохранённое при переносе; не используется.

После переноса новый путь к kubeconfig:

```bash
export KUBECONFIG=/Users/andrejfaddeev/enjoyers/lab3/ansible/.state/lab3/kubeconfig
kubectl -n shop get pods,svc,pvc
```

Если в открытом терминале был задан старый KUBECONFIG, выполните export заново.
Сам кластер и данные PostgreSQL при переносе файлов не изменялись.
