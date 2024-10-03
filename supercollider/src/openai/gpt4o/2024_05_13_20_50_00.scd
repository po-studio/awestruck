// Start the server
s.waitForBoot {
    // Define a SynthDef for a drone sound
    SynthDef(\drone, {
        |out = 0, freq = 110, amp = 0.4, pan = 0, attack = 0.5, release = 2.0|

        var env = EnvGen.kr(
            Env.perc(attack, release), doneAction: 2
        );

        var osc1 = LFTri.ar(freq, 0, 0.4);
        var osc2 = LFSaw.ar(freq * 0.5, 0, 0.4);
        var osc3 = SinOsc.ar(freq * 1.5, 0, 0.4);

        var mix = Mix.new([osc1, osc2, osc3]) * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for an evolving texture
    SynthDef(\texture, {
        |out = 0, freq = 440, amp = 0.2, pan = 0, attack = 0.01, decay = 2.0, sustain = 1.0, release = 2.0|

        var env = EnvGen.kr(
            Env.adsr(attack, decay, sustain, release), doneAction: 2
        );

        var noise = PinkNoise.ar * 0.5;
        var filter = RLPF.ar(noise, XLine.kr(500, 2000, 30), 0.2);

        var osc = SinOsc.ar(freq * [0.99, 1.01], 0, 0.3);
        var mix = Mix.new([filter, osc]) * env * amp;
        Out.ar(out, Pan2.ar(mix, pan));
    }).add;

    // Define a SynthDef for subtle rhythmic clicks
    SynthDef(\clicks, {
        |out = 0, amp = 0.1, pan = 0|

        var click = Dust.kr(10) * amp;
        var env = Decay.kr(click, 0.01);
        var signal = SinOsc.ar(2000) * env;
        Out.ar(out, Pan2.ar(signal, pan));
    }).add;

    // Patterns to generate the music
    Pdef(\drone, Pbind(
        \instrument, \drone,
        \dur, Pseq([16, 8, 12, 20], inf),
        \freq, Pseq([110, 220, 165, 330], inf),
        \amp, 0.4,
        \attack, 1.0,
        \release, 4.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\texture, Pbind(
        \instrument, \texture,
        \dur, Pseq([8, 12, 16, 20], inf),
        \freq, Pseq([440, 550, 660, 770], inf),
        \amp, 0.2,
        \attack, 0.05,
        \decay, 4.0,
        \sustain, 1.0,
        \release, 4.0,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;

    Pdef(\clicks, Pbind(
        \instrument, \clicks,
        \dur, Pseq([0.25, 0.5, 0.75, 1], inf),
        \amp, 0.1,
        \pan, Pwhite(-1.0, 1.0, inf)
    )).play;
};
