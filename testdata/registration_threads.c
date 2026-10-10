#include <stdint.h>
#if defined(_WIN32)
#include <windows.h>
#define EXPORT __declspec(dllexport)
static HANDLE thread;
#else
#include <pthread.h>
#define EXPORT
static pthread_t thread;
#endif
typedef struct { int32_t a,b; } Pair;
static const Pair *saved_pair;
static int32_t (*saved_callback)(int32_t,int32_t);
static int started;
static int32_t result;

#if defined(_WIN32)
static DWORD WINAPI run_worker(void *unused)
#else
static void *run_worker(void *unused)
#endif
{
    (void)unused;
    result=saved_callback(saved_pair->a,saved_pair->b);
    return 0;
}

EXPORT int32_t registration_start(const Pair *pair, int32_t (*callback)(int32_t,int32_t)) {
    if (started || !pair || !callback) return -1;
    saved_pair=pair;
    saved_callback=callback;
    result=0;
#if defined(_WIN32)
    thread=CreateThread(0,0,run_worker,0,0,0);
    if (!thread) return -1;
#else
    if (pthread_create(&thread,0,run_worker,0)) return -1;
#endif
    started=1;
    return 0;
}

EXPORT int32_t registration_join(void) {
    if (!started) return 0;
#if defined(_WIN32)
    if (WaitForSingleObject(thread,INFINITE)!=WAIT_OBJECT_0) return -1;
    CloseHandle(thread);
    thread=0;
#else
    if (pthread_join(thread,0)) return -1;
#endif
    started=0;
    saved_pair=0;
    saved_callback=0;
    return result;
}
