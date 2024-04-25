(
s.waitForBoot({
    play {
        var gain = 0.25;  // Set the output gain

        Resonz.ar(
            // Resonant filter applied to a stereo Crackle noise source
            Crackle.ar ! 2,  // Generate two channels of Crackle noise
            // Control frequency of the resonant filter using a duty cycle generator
            Duty.kr(
                Dseq([1, 1, 4, 2, 2] / 8, inf),  // Sequence of rates for the duty cycle
                0,  // Phase of the duty cycle
                Dseq([99, Dwhite(99, 9000, 1)], inf)  // Sequence combining a fixed value and random values
            ),
            // Exponential random filter bandwidth, re-triggered at a regular interval
            TExpRand.kr(0.001, 1, Impulse.kr(8))  // Triggered every 1/8th of a second
        ) * 4 * gain;  // Apply the gain after multiplying the output by 4
    }
});
)
