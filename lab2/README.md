init

Перед развёртыванием системы мониторинга была проверена готовность рабочей среды. Были проверены доступность Docker Engine, наличие Helm и подключение к локальному кластеру Kubernetes, созданному с помощью kind. Было проверено состояние узла локального кластера Kubernetes kind-lab2. Узел lab2-control-plane находился в состоянии Ready и выполнял роль control-plane. Версия Kubernetes на узле составляла v1.37.0.

![alt text](screenshots/image.png)
Рисунок 1 — Проверка версий клиента и сервера Docker

![alt text](screenshots/image-1.png)
Рисунок 2 — Список локальных кластеров kind

![alt text](screenshots/image-2.png)
Рисунок 3 — Проверка версии Helm

![alt text](screenshots/image-3.png)
Рисунок 4 — Проверка текущего контекста kubectl

![alt text](screenshots/image-4.png)
Рисунок 5 — Проверка готовности узла кластера kind-lab2

