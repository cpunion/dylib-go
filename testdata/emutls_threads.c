#include <stdint.h>
#include <stdatomic.h>
#if defined(_WIN32)
#include <windows.h>
#define EXPORT __declspec(dllexport)
#else
#include <pthread.h>
#include <time.h>
#define EXPORT
#endif

typedef int (*Step)(int, int);
typedef struct { Step step; int result; } Worker;
static int result;
static atomic_int paused, ready, release_workers;

#if defined(_WIN32)
static DWORD WINAPI worker(void *p)
#else
static void *worker(void *p)
#endif
{
    Worker *w = (Worker *)p;
    for (int i = 0; i < 100; ++i) {
        if (w->step(20, 22) != 49 + i) { w->result = -1; break; }
    }
    // No raw code or descriptor reference remains while the thread waits.
    w->step = 0;
    atomic_fetch_add_explicit(&ready, 1, memory_order_release);
    while (atomic_load_explicit(&paused, memory_order_relaxed) && !atomic_load_explicit(&release_workers, memory_order_acquire)) {
#if defined(_WIN32)
        Sleep(1);
#else
        struct timespec delay = {0, 1000000};
        nanosleep(&delay, 0);
#endif
    }
    return 0;
}

static Worker workers[4];
static int started;
#if defined(_WIN32)
static HANDLE threads[4];
#else
static pthread_t threads[4];
#endif
static void start_workers(const void *entry) {
    started = 0;
    result = 0;
    atomic_store(&ready, 0);
    atomic_store(&release_workers, 0);
    for (int i = 0; i < 4; ++i) {
        workers[i].step = (Step)entry;
        workers[i].result = 0;
#if defined(_WIN32)
        threads[i] = CreateThread(0, 0, worker, &workers[i], 0, 0);
        if (!threads[i]) { result = -2; break; }
#else
        if (pthread_create(&threads[i], 0, worker, &workers[i])) { result = -2; break; }
#endif
        ++started;
    }
}
static void join_workers(void) {
    atomic_store_explicit(&release_workers, 1, memory_order_release);
    for (int i = 0; i < started; ++i) {
#if defined(_WIN32)
        if (WaitForSingleObject(threads[i], INFINITE) != WAIT_OBJECT_0) result = -3;
        CloseHandle(threads[i]);
#else
        if (pthread_join(threads[i], 0)) result = -3;
#endif
        if (workers[i].result) result = workers[i].result;
    }
    started = 0;
}
EXPORT void tls_threads(const void *entry) { atomic_store(&paused, 0); start_workers(entry); join_workers(); }
EXPORT void tls_threads_pause(const void *entry) { atomic_store(&paused, 1); start_workers(entry); }
EXPORT int tls_threads_ready(int a, int b) { (void)a; (void)b; return atomic_load_explicit(&ready, memory_order_acquire); }
EXPORT int tls_threads_release(int a, int b) { (void)a; (void)b; join_workers(); return result; }
EXPORT int tls_thread_result(int a, int b) { (void)a; (void)b; return result; }
