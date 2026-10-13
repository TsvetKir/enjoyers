# Восстановление мониторинга второй лабораторной

`lab2.values.yaml` — копия исходной конфигурации из второй лабораторной.
`lab3.values.yaml` — настройки для текущего кластера и политик безопасности.
Chart kube-prometheus-stack закреплён на версии 92.3.0. Релиз — `monitoring`,
namespace — `monitoring`. Исторические метрики со старой машины не восстановлены.

Пять проверок Kyverno сохранены. Только политика источников образов разделена:
для обычных workload остаётся `ghcr.io/*`, для monitoring разрешены восемь
конкретных образов с тегами из chart. Список включает config-reloader,
создаваемый Prometheus Operator. При смене версии chart список нужно пересмотреть.
Теги фиксируют версию, но не являются проверкой цифровой подписи или digest.

В настройках указаны CPU/memory requests и limits, обязательные метки,
non-root и privileged=false. Grafana initChownData отключён; права тома
задаются через fsGroup. Node Exporter не установлен: на этом этапе нужны метрики
приложения и Kubernetes-объектов, а не системные метрики узла.
Loki, Jaeger, Alloy, Karma и webhook второй лабораторной сейчас не восстанавливаются.
Ссылки на отсутствующие Loki и webhook отключены; внешние уведомления не отправляются.

Запуск только этого этапа в уже подготовленном кластере:

```bash
cd ../ansible
source .venv/bin/activate
ansible-playbook monitoring.yml
```

Основной playbook тоже включает этот этап (`monitoring_enabled: true`).
ServiceMonitor и PrometheusRule приложения должны иметь метку `release: monitoring`.
Сбор метрик shop, дашборд приложения и три алерта являются следующим этапом.

Проверка:

```bash
export KUBECONFIG=/Users/andrejfaddeev/enjoyers/lab3/ansible/.state/lab3/kubeconfig
kubectl -n monitoring get pods,pvc
kubectl -n monitoring port-forward svc/monitoring-kube-prometheus-prometheus 9090:9090
```

Открыть http://localhost:9090. Grafana: сервис monitoring-grafana, порт 80;
для локального доступа использовать `port-forward svc/monitoring-grafana 3000:80`.
Пароль хранится в Secret monitoring-grafana (ключ admin-password); не публиковать
его в отчёте или Git. Доступ к Alertmanager: сервис monitoring-kube-prometheus-alertmanager,
порт 9093. Названия проверяются после установки.

## Проверено 12 октября 2026

Ansible monitoring.yml: failed=0; все пять подов мониторинга Ready, PVC Grafana
и Prometheus Bound. HTTP readiness Prometheus/Alertmanager и /api/health Grafana
вернули 200; Grafana сообщила database=ok. Prometheus: 17 целей, 13 up.
Четыре цели недоступны: kube-controller-manager, kube-etcd, kube-proxy,
kube-scheduler. Их сетевые/метрические endpoints в kind отдельно не настраивались.
Эти цели не являются метриками shop; dashboard/алерты shop пока не добавлены.

Server-side dry-run (без создания Pod): одобренный образ в monitoring принят,
неразрешённый образ, privileged, root, отсутствие ресурсов и меток отклонены
политиками. Исключение namespace из пяти политик НЕ применялось.

## Метрики приложения shop

В API и worker добавлен GET /metrics. Использована официальная Go-библиотека
Prometheus client_golang v1.20.5, совместимая с Go 1.23 в текущих Dockerfile.
API экспортирует shop_http_requests_total (route, method, code) и
shop_http_request_duration_seconds (гистограмма длительности).
Набор route ограничен /health, /order, /orders и other; произвольные URL не создают
новые серии. Сбор /metrics не учитывается как пользовательская нагрузка.
При анализе бизнес-запросов следует исключать route=/health, который вызывают probes.
Worker экспортирует shop_worker_processed_orders_total после успешного commit
и shop_worker_errors_total с operation=schema или process. Счётчики относятся
к процессу и сбрасываются при его перезапуске, поэтому для скорости используются rate/increase.
Также доступны стандартные метрики Go и процесса.

В chart добавлен шаблон templates/servicemonitor.yaml, создающий два ServiceMonitor.
API выбирается по меткам Service и порту http, worker — по порту health.
Scrape идёт по /metrics каждые 15 секунд. Метка release=monitoring позволяет
Prometheus найти оба монитора. Название health у порта worker не задаёт HTTP-путь.

В values мониторинг выключен по умолчанию, чтобы chart можно было отрендерить
без установленного Prometheus Operator. Ansible передаёт monitoring.enabled=true
и сначала устанавливает мониторинг, затем shop. При отключении monitoring_enabled
создание ServiceMonitor выключается; существующий стек не удаляется автоматически.

Проверка в Prometheus после обновления:

```promql
up{namespace="shop", service=~"shop-api|shop-worker"}
```

Ожидается пять серий со значением 1: три API и два worker.
Дашборд и PrometheusRule приложения на этом этапе ещё не добавлены.

### Проверка подключения shop

После обновления приложения Ansible завершился с ok=80, failed=0.
API Prometheus показал пять активных целей shop со статусом up (3 API + 2 worker).
Пять GET /orders дали shop_http_requests_total{route="/orders"}=5 на обработавшей
их реплике. Счётчик обработанных заказов worker присутствовал у обеих реплик
со значением 0; новые заказы в ходе этой проверки не создавались.
Сразу после rollout могут кратковременно оставаться серии up=0 старых подов.
Для сводной проверки готовых реплик используется:

```promql
sum by (service) (up{namespace="shop",service=~"shop-api|shop-worker"})
```

Ожидаемые значения: shop-api=3, shop-worker=2.

## Дашборд и три алерта

В chart добавлены `templates/grafana-dashboard.yaml`, `files/shop-dashboard.json`
и `templates/prometheusrule.yaml`. Дашборд распространяется через ConfigMap с
меткой grafana_dashboard=1; sidecar Grafana загружает его из namespace shop.
UID при текущих настройках: shop-shop-overview. Изменения JSON нужно вносить в Git,
поскольку дашборд предоставляется из файла. На нём восемь панелей: доступные API
и worker, число firing-алертов shop, частота запросов, доля 5xx, p95 задержки,
скорость обработки заказов и ошибки worker по операциям.

Три правила в PrometheusRule имеют метку release=monitoring:

| Алерт | Условие по умолчанию | Обоснование |
| --- | --- | --- |
| ShopApiUnavailable | Нет успешных целей API либо серии up отсутствуют в течение 1 минуты | Обнаруживает потерю сбора метрик со всех реплик, включая удаление всех endpoints. Это не самостоятельная проверка работоспособности бизнес-операций. |
| ShopApiHighErrorRate | Доля 5xx >10% по /order и /orders за 2 минуты; минимум 20 запросов; условие сохраняется 2 минуты | Позволяет обнаружить отказ операций API, например при проблемах с PostgreSQL, даже когда /health продолжает отвечать. |
| ShopApiHighLatency | p95 >1 секунды за 2 минуты; минимум 20 запросов; условие сохраняется 2 минуты | Обнаруживает деградацию задержки до полного отказа. |

Health probes исключены из бизнес-метрик, чтобы частые успешные /health не скрывали
ошибки заказов. Ошибки и задержки суммируются по всем репликам. Порог минимальной
нагрузки снижает влияние единичных запросов. При отсутствии трафика эти два правила
не срабатывают; за отсутствие scrape отвечает ShopApiUnavailable. Поле `for`
задаёт время непрерывного выполнения условия перед переходом Pending → Firing.
Пороги и интервалы задаются в monitoring.alerts файла values.yaml.

На текущем этапе выполнена установка правил. Управляемый опыт со срабатыванием
и последующим восстановлением приложения проводится отдельным следующим этапом.
Внешняя доставка уведомлений отключена; состояние алертов проверяется в
Prometheus и Alertmanager, без отправки сообщений сторонним получателям.

Проверка 13 октября 2026: helm lint и promtool check rules прошли (3 правила).
Helm-релиз shop обновлён до revision 4. API Grafana подтвердил загрузку восьми
панелей дашборда shop-shop-overview; запросы всех панелей успешно выполнены через
API Prometheus. Три правила имели health=ok, state=inactive. Переход в Firing
в рамках этого этапа не проверялся.
