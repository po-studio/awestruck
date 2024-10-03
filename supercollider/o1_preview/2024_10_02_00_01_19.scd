s.options.numInputBusChannels = 0;
s.options.numOutputBusChannels = 2;
s.reboot;

s.waitForBoot {
    // Define the path to save the SynthDef file
    var synthDefPath = thisProcess.nowExecutingPath.dirname.withTrailingSlash ++ "../synthdefs/";

    // Ensure the directory exists
  // synthDefPath.standardizePath.mkdirTree;

    // Define and write the combined SynthDef
    SynthDef.new(\combinedSynth, {
        |out = 0, amp = 0.5|
        var padEnv, padOsc, padFreqSeq, padFreq, padTrig;
        var rhythmEnv, rhythmNoise, rhythmFreqSeq, rhythmFreq, rhythmTrig;
        var melodyEnv, melodyOsc, melodyFreqSeq, melodyFreq, melodyTrig;
        var combinedMix;

        // Common amplitude control
        amp = amp * 0.5; // Reduce overall amplitude to prevent clipping

        // Evolving Pad
        // Frequency sequence for the pad
        padFreqSeq = Dseq([220, 440, 330, 550, 660], inf);
        padTrig = Impulse.kr(1 / Dseq([8, 4, 2], inf)); // Triggers at varying durations
        padFreq = Demand.kr(padTrig, 0, padFreqSeq);
        padEnv = EnvGen.kr(Env.adsr(0.05, 2.0, 0.8, 2.0), padTrig);
        padOsc = Mix.new([
            LFSaw.kr(padFreq * [0.99, 1.01], 0, 0.2).sum,
            SinOsc.ar(padFreq * [1.01, 0.99], 0, 0.2).sum,
            Pulse.ar(padFreq * [0.5, 1.5], 0.5, 0.2).sum
        ]) * padEnv * amp;
        padOsc = Pan2.ar(padOsc, LFNoise1.kr(0.1));

        // Rhythm
        rhythmFreqSeq = Dseq([100, 200, 300, 400, 500], inf);
        rhythmTrig = Impulse.kr(1 / Dseq([0.5, 0.25, 0.75, 1], inf));
        rhythmFreq = Demand.kr(rhythmTrig, 0, rhythmFreqSeq);
        rhythmEnv = EnvGen.kr(Env.perc(0.01, 0.2), rhythmTrig);
        rhythmNoise = BPF.ar(WhiteNoise.ar * rhythmEnv, rhythmFreq, 0.2) * amp * 0.2;
        rhythmNoise = Pan2.ar(rhythmNoise, LFNoise1.kr(0.5));

        // Melody
        melodyFreqSeq = Dseq([660, 880, 990, 770, 1100], inf);
        melodyTrig = Impulse.kr(1 / Dseq([1, 0.5, 0.75, 1.5, 2], inf));
        melodyFreq = Demand.kr(melodyTrig, 0, melodyFreqSeq);
        melodyEnv = EnvGen.kr(Env.adsr(0.02, 0.5, 0.3, 0.7), melodyTrig);
        melodyOsc = Pulse.ar(melodyFreq * [0.5, 1, 2], 0.5, 0.2).sum;
        melodyOsc = RLPF.ar(melodyOsc, XLine.kr(2000, 400, 10), 0.3);
        melodyOsc = melodyOsc * melodyEnv * amp * 0.6;
        melodyOsc = Pan2.ar(melodyOsc, LFNoise1.kr(0.2));

        // Combine all elements
        combinedMix = padOsc + rhythmNoise + melodyOsc;

        // Output
        Out.ar(out, combinedMix);
    }).writeDefFile(synthDefPath);
};
