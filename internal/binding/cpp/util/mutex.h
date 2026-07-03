// Copyright 2007 The RE2 Authors.  All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#ifndef UTIL_MUTEX_H_
#define UTIL_MUTEX_H_

/*
 * A simple mutex wrapper, supporting locks and read-write locks.
 * You should assume the locks are *not* re-entrant.
 */

#ifdef RE2_NO_THREADS
#include <assert.h>
#define MUTEX_IS_LOCK_COUNTER
#else
#ifdef _WIN32
// Requires Windows Vista or Windows Server 2008 at minimum.
#include <windows.h>
#if defined(WINVER) && WINVER >= 0x0600
#define MUTEX_IS_WIN32_SRWLOCK
#endif
#else
#ifndef _POSIX_C_SOURCE
#define _POSIX_C_SOURCE 200809L
#endif
#include <unistd.h>
#if defined(_POSIX_READER_WRITER_LOCKS) && _POSIX_READER_WRITER_LOCKS > 0
#define MUTEX_IS_PTHREAD_RWLOCK
#endif
#endif
#endif

#if defined(MUTEX_IS_LOCK_COUNTER)
typedef int MutexType;
#elif defined(MUTEX_IS_WIN32_SRWLOCK)
typedef SRWLOCK MutexType;
#elif defined(MUTEX_IS_PTHREAD_RWLOCK)
#include <pthread.h>
#include <stdexcept>
#include <stdlib.h>
typedef pthread_rwlock_t MutexType;
#else
#include <shared_mutex>
typedef std::shared_mutex MutexType;
#endif

namespace re2 {

class Mutex {
public:
    inline Mutex();
    inline ~Mutex();
    inline void Lock();   // Block if needed until free then acquire exclusively
    inline void Unlock(); // Release a lock acquired via Lock()
    // Note that on systems that don't support read-write locks, these may
    // be implemented as synonyms to Lock() and Unlock().  So you can use
    // these for efficiency, but don't use them anyplace where being able
    // to do shared reads is necessary to avoid deadlock.
    inline void ReaderLock();                // Block until free or shared then acquire a share
    inline void ReaderUnlock();              // Release a read share of this Mutex
    inline void WriterLock() { Lock(); }     // Acquire an exclusive lock
    inline void WriterUnlock() { Unlock(); } // Release a lock from WriterLock()

private:
    MutexType mutex_;

    // Catch the error of writing Mutex when intending MutexLock.
    Mutex(Mutex *ignored);

    Mutex(const Mutex &) = delete;
    Mutex &operator=(const Mutex &) = delete;
};

#if defined(MUTEX_IS_LOCK_COUNTER)

Mutex::Mutex() : mutex_(0) {}
Mutex::~Mutex() { assert(mutex_ == 0); }
void Mutex::Lock() { assert(--mutex_ == -1); }
void Mutex::Unlock() { assert(mutex_++ == -1); }
void Mutex::ReaderLock() { assert(++mutex_ > 0); }
void Mutex::ReaderUnlock() { assert(mutex_-- > 0); }

#elif defined(MUTEX_IS_WIN32_SRWLOCK)

Mutex::Mutex() : mutex_(SRWLOCK_INIT) {}
Mutex::~Mutex() {}
void Mutex::Lock() { AcquireSRWLockExclusive(&mutex_); }
void Mutex::Unlock() { ReleaseSRWLockExclusive(&mutex_); }
void Mutex::ReaderLock() { AcquireSRWLockShared(&mutex_); }
void Mutex::ReaderUnlock() { ReleaseSRWLockShared(&mutex_); }

#elif defined(MUTEX_IS_PTHREAD_RWLOCK)

#define SAFE_PTHREAD(fncall)                                                                                                                         \
    do {                                                                                                                                             \
        if ((fncall) != 0)                                                                                                                           \
            throw std::runtime_error("RE2 pthread failure");                                                                                         \
    } while (0);

Mutex::Mutex() { SAFE_PTHREAD(pthread_rwlock_init(&mutex_, NULL)); }
Mutex::~Mutex() { pthread_rwlock_destroy(&mutex_); }
void Mutex::Lock() { SAFE_PTHREAD(pthread_rwlock_wrlock(&mutex_)); }
void Mutex::Unlock() { SAFE_PTHREAD(pthread_rwlock_unlock(&mutex_)); }
void Mutex::ReaderLock() { SAFE_PTHREAD(pthread_rwlock_rdlock(&mutex_)); }
void Mutex::ReaderUnlock() { SAFE_PTHREAD(pthread_rwlock_unlock(&mutex_)); }

#undef SAFE_PTHREAD

#else

Mutex::Mutex() {}
Mutex::~Mutex() {}
void Mutex::Lock() { mutex_.lock(); }
void Mutex::Unlock() { mutex_.unlock(); }
void Mutex::ReaderLock() { mutex_.lock_shared(); }
void Mutex::ReaderUnlock() { mutex_.unlock_shared(); }

#endif

// --------------------------------------------------------------------------
// Some helper classes

// MutexLock(mu) acquires mu when constructed and releases it when destroyed.
class MutexLock {
public:
    explicit MutexLock(Mutex *mu) : mu_(mu) { mu_->Lock(); }
    ~MutexLock() { mu_->Unlock(); }

private:
    Mutex *const mu_;

    MutexLock(const MutexLock &) = delete;
    MutexLock &operator=(const MutexLock &) = delete;
};

// ReaderMutexLock and WriterMutexLock do the same, for rwlocks
class ReaderMutexLock {
public:
    explicit ReaderMutexLock(Mutex *mu) : mu_(mu) { mu_->ReaderLock(); }
    ~ReaderMutexLock() { mu_->ReaderUnlock(); }

private:
    Mutex *const mu_;

    ReaderMutexLock(const ReaderMutexLock &) = delete;
    ReaderMutexLock &operator=(const ReaderMutexLock &) = delete;
};

class WriterMutexLock {
public:
    explicit WriterMutexLock(Mutex *mu) : mu_(mu) { mu_->WriterLock(); }
    ~WriterMutexLock() { mu_->WriterUnlock(); }

private:
    Mutex *const mu_;

    WriterMutexLock(const WriterMutexLock &) = delete;
    WriterMutexLock &operator=(const WriterMutexLock &) = delete;
};

// Catch bug where variable name is omitted, e.g. MutexLock (&mu);
#define MutexLock(x) static_assert(false, "MutexLock declaration missing variable name")
#define ReaderMutexLock(x) static_assert(false, "ReaderMutexLock declaration missing variable name")
#define WriterMutexLock(x) static_assert(false, "WriterMutexLock declaration missing variable name")

} // namespace re2

#endif // UTIL_MUTEX_H_
