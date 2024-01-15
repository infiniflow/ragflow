import collections
import time


class MOTTimer(object):
    """
    This class used to compute and print the current FPS while evaling.
    """

    def __init__(self, window_size=20):
        self.start_time = 0.
        self.diff = 0.
        self.duration = 0.
        self.deque = collections.deque(maxlen=window_size)

    def tic(self):
        # using time.time instead of time.clock because time time.clock
        # does not normalize for multithreading
        self.start_time = time.time()

    def toc(self, average=True):
        self.diff = time.time() - self.start_time
        self.deque.append(self.diff)
        if average:
            self.duration = np.mean(self.deque)
        else:
            self.duration = np.sum(self.deque)
        return self.duration

    def clear(self):
        self.start_time = 0.
        self.diff = 0.
        self.duration = 0.
