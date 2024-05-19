// Start the server
s.waitForBoot {
    // Define a SynthDef for a complex evolving pad
    SynthDef(\evolvingPad, {
        |out = 0, freq = 440, amp = 0.5, pan = 0, attack = 0.01, decay = 1.0, sustain = 0.7, release = 1.0|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var osc1 = LFSaw.kr(freq * [0.99, 1.01], 0, 0.2).sum;
        var osc2 = SinOsc.ar(freq * [1.01, 0.99], 0, 0.2).sum;
        var osc3 = Pulse.ar(freq * [0.5, 1.5], 0.5, 0.2).sum;

        var mix = Mix.new([osc1, osc2, osc3]) * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a rhythmic percussive element
    SynthDef(\rhythm, {
        |out = 0, freq = 200, amp = 0.5, pan = 0, attack = 0.01, decay = 0.2, sustain = 0.1, release = 0.3|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var noise = WhiteNoise.ar * env * amp;
        var filter = BPF.ar(noise, freq, 0.2);
        var mix = Mix.new(filter);
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for a melodic element
    SynthDef(\melody, {
        |out = 0, freq = 440, amp = 0.3, pan = 0, attack = 0.01, decay = 0.5, sustain = 0.2, release = 0.7|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var osc = Pulse.ar(freq * [0.5, 1, 2], 0.5, 0.2).sum;
        var filter = RLPF.ar(osc, XLine.kr(2000, 400, 10), 0.3);
        var mix = Mix.new(filter) * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Patterns to generate the music
    Pdef(\music, Pbind(
        \instrument, \evolvingPad,
        \dur, Pseq([8, 4, 2], inf),
        \freq, Pseq([220, 440, 330, 550, 660], inf),
        \amp, 0.2,
        \attack, 0.05,
        \decay, 2.0,
        \sustain, 0.8,
        \release, 2.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\rhythm, Pbind(
        \instrument, \rhythm,
        \dur, Pseq([0.5, 0.25, 0.75, 1], inf),
        \freq, Pseq([100, 200, 300, 400, 500], inf),
        \amp, 0.1,
        \attack, 0.01,
        \decay, 0.1,
        \sustain, 0.2,
        \release, 0.2,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\melody, Pbind(
        \instrument, \melody,
        \dur, Pseq([1, 0.5, 0.75, 1.5, 2], inf),
        \freq, Pseq([660, 880, 990, 770, 1100], inf),
        \amp, 0.2,
        \attack, 0.02,
        \decay, 0.5,
        \sustain, 0.3,
        \release, 0.7,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;
};
