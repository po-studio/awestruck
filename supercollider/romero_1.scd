(
s.waitForBoot({
    play {
        var gain = 0.1;  // Master output gain

        GVerb.ar(
            // Apply GVerb reverb to the summed output of Blip oscillators
            Blip.ar(
                // Blip oscillators with frequency modulation
                (Duty.kr(
                    1 / [1, 2, 4],  // Control rate duty cycle with varying frequencies
                    0,  // Phase offset
                    Dseq(
                        [0, 3, 7, 12, 17] + 24,  // Midi note numbers sequence, shifted up by 24 semitones
                        inf  // Repeat infinitely
                    )
                ).midicps * [1, 4, 8]),  // Convert MIDI notes to frequency and modulate harmonics
                LFNoise1.kr(1 / 4, 3, 4)  // Control rate noise for number of harmonics
            ).sum * gain,  // Sum the outputs and apply gain
            200,  // Room size for the reverb
            8     // Damping factor
        )
    }
});
)
