#include <stdint.h>
#include <stdlib.h>
#if defined(_WIN32)
#include <windows.h>
#define EXPORT __declspec(dllexport)
#else
#include <pthread.h>
#define EXPORT
#endif
struct worker {
    int32_t (*callback)(int32_t,int32_t);
    int32_t result;
};
#if defined(_WIN32)
static DWORD WINAPI run_worker(void *data)
#else
static void *run_worker(void *data)
#endif
{
    struct worker *worker = (struct worker *)data;
    for (int i=0; i<10; i++) worker->result += worker->callback(20,22);
    return 0;
}
EXPORT int32_t invoke_threads(int32_t (*fn)(int32_t,int32_t), int32_t n) {
    if (n<1 || n>8) return -1;
    struct worker workers[8] = {0};
    int started=0;
#if defined(_WIN32)
    HANDLE threads[8];
#else
    pthread_t threads[8];
#endif
    for (int i=0; i<n; i++) {
        workers[i].callback=fn;
#if defined(_WIN32)
        threads[i]=CreateThread(0,0,run_worker,&workers[i],0,0);
        if (!threads[i]) break;
#else
        if (pthread_create(&threads[i],0,run_worker,&workers[i])) break;
#endif
        started++;
    }
    int32_t total=0;
    for (int i=0; i<started; i++) {
#if defined(_WIN32)
        WaitForSingleObject(threads[i],INFINITE);
        CloseHandle(threads[i]);
#else
        pthread_join(threads[i],0);
#endif
        total+=workers[i].result;
    }
    return started==n ? total : -1;
}
