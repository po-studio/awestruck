(
s.waitForBoot({
    play {
        var gain = 0.25;  // Master output gain
        
        MoogFF.ar(
            // Filtered signal using MoogFF
            LFTri.ar(
                // Low-frequency triangle oscillator modulating a comb filter
                CombN.ar(
                    // Comb filter applied to a complex duty cycle generator
                    Duty.ar(
                        1 / 8,  // Frequency of duty cycle
                        0,  // Phase of the duty cycle
                        Dseq(
                            Dshuf(
                                List.fib(16) % 8 * 99,  // Fibonacci sequence modulated and shuffled
                                8  // Shuffling pieces
                            ),
                            inf  // Infinite repeats
                        )
                    ),
                    4,  // Delay time for CombN
                    4,  // Maximum delay time for CombN
                    16  // Decay time of the comb filter
                )
            ) / 4,  // Scale down the oscillator output
            LFTri.kr(1 / 16, 0, 2000, 3000)  // Control rate LFTri oscillator for cutoff frequency modulation
        ) ! 2 * gain;  // Duplicate the signal for stereo output and apply gain
    }
});
)
