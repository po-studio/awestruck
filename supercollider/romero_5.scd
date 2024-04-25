(
s.waitForBoot({
    // Allocate 8 consecutive buffers with varying sine wave contents
    Buffer.allocConsecutive(8, s, 1024, 1, {|b, i|
        b.sine1Msg(1 / (1..((i + 1) * 6)))  // Fill each buffer with a sine wave where the frequency decreases
    }, 0);

    // Play block
    {
        var f = [50, 75, 99];  // Frequency array for VOsc3
        // Vector oscillator with three frequency components modulated by noise and lagged slightly
        VOsc3.ar(
            LFNoise1.kr(1 / 4, 3, 4),  // Low-frequency noise as frequency modulator
            *f  // Spread the frequencies as arguments to VOsc3
        ).lag(3e-3) ! 2  // Apply a small lag to the output and duplicate for stereo
    }.play;
});
)
