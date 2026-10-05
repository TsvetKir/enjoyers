#!/bin/bash
set -e

CGROUP_DIR="/sys/fs/cgroup/mydocker"
echo "Запуск MyDocker..."

sudo rmdir $CGROUP_DIR 2>/dev/null || true
sudo mkdir -p $CGROUP_DIR
echo "50M" | sudo tee $CGROUP_DIR/memory.max > /dev/null
echo "50000 100000" | sudo tee $CGROUP_DIR/cpu.max > /dev/null
echo "100" | sudo tee $CGROUP_DIR/pids.max > /dev/null  # Увеличили до 100, чтобы не блокировать сам сервис

start_container() {
    mount -t proc proc /proc
    
    ip link set lo up
    
    hostname mydocker-host
    
    echo "Внутри контейнера: применение прав и запуск сервиса..."
    
    ./seccomp_wrapper ./myservice
    
    wait
}

export -f start_container

unshare -U -r -p -f -m -n -u -i bash -c start_container &
UNSHARE_PID=$!


echo "Ожидание запуска сервиса..."
sleep 3

REAL_PID=$(pgrep -f "myservice" | head -n 1)

if [ -n "$REAL_PID" ]; then
    echo "Хост: найден реальный PID $REAL_PID на хосте"
    echo "Хост: перемещаю PID $REAL_PID в cgroup для ограничения ресурсов..."
    echo $REAL_PID | sudo tee $CGROUP_DIR/cgroup.procs > /dev/null
    echo "MyDocker успешно запущен!"
else
    echo "Ошибка: не удалось найти процесс myservice на хосте."
    exit 1
fi

wait $UNSHARE_PID
