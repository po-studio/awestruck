(
s.waitForBoot({
    (
        play {
            var gain = 0.25;
            var b = LocalBuf(5000, 2).clear;
            var i = Sweep.ar(BufRd.ar(2, b, Saw.ar(50, 2000, 5000)), 60000);
            BufWr.ar(Saw.ar([4, 3]), b, i);
            BufRd.ar(2, b, i) / 6 * gain;
        }
    );
});
)
