#if defined(_WIN32)
#define EXPORT __declspec(dllexport)
#else
#define EXPORT
#endif
static int events[128];
static int count;
EXPORT void record_event(int value) { if (count < 128) events[count++] = value; }
EXPORT int recorded_event(int index, int unused) {
    (void)unused;
    if (index < 0) return count;
    return index < count ? events[index] : -1;
}
