#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>
#include <seccomp.h>
#include <sys/syscall.h>

int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <command> [args...]\n", argv[0]);
        return 1;
    }

    // 1. Инициализируем контекст seccomp. По умолчанию разрешаем всё (SCMP_ACT_ALLOW)
    scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
    if (ctx == NULL) {
        perror("seccomp_init failed");
        return 1;
    }

    // 2. Добавляем правило: блокировать mkdir и mkdirat, возвращая EPERM (Operation not permitted)
    if (seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(mkdir), 0) < 0) {
        perror("seccomp_rule_add mkdir failed");
        seccomp_release(ctx);
        return 1;
    }
    
    if (seccomp_rule_add(ctx, SCMP_ACT_ERRNO(EPERM), SCMP_SYS(mkdirat), 0) < 0) {
        perror("seccomp_rule_add mkdirat failed");
        seccomp_release(ctx);
        return 1;
    }

    // 3. Загружаем фильтр в ядро. После этого изменить его уже нельзя
    if (seccomp_load(ctx) < 0) {
        perror("seccomp_load failed");
        seccomp_release(ctx);
        return 1;
    }

    // Очищаем контекст (фильтр уже в ядре)
    seccomp_release(ctx);

    // 4. Выполняем целевую команду. Если мы дошли сюда, фильтр уже работает
    execvp(argv[1], &argv[1]);
    
    // Если execvp вернул управление, значит произошла ошибка запуска
    perror("execvp failed");
    return 1;
}
