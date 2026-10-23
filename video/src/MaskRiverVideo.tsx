import React from 'react';
import {AbsoluteFill, Audio, Sequence, staticFile} from 'remotion';
import {COLORS} from './styles/tokens';
import {Scene1Problem} from './scenes/Scene1Problem';
import {Scene2Workflow} from './scenes/Scene2Workflow';
import {Scene3Walkthrough} from './scenes/Scene3Walkthrough';
import {Scene4Streaming} from './scenes/Scene4Streaming';
import {Scene5Recovery} from './scenes/Scene5Recovery';
import {Scene6Closing} from './scenes/Scene6Closing';
import {Subtitles} from './components/Subtitles';
import {OpeningClosing, TransitionAccents} from './components/Transitions';

/** Absolute scene windows at 30fps (frames): 750 / 1200 / 2550 / 1500 / 1350 / 450 = 7800 */
export const SCENES = [
  {id: 'scene1', from: 0, duration: 750},
  {id: 'scene2', from: 750, duration: 1200},
  {id: 'scene3', from: 1950, duration: 2550},
  {id: 'scene4', from: 4500, duration: 1500},
  {id: 'scene5', from: 6000, duration: 1350},
  {id: 'scene6', from: 7350, duration: 450},
] as const;

export const MaskRiverVideo: React.FC = () => {
  return (
    <AbsoluteFill style={{backgroundColor: COLORS.bgDeep}}>
      <Sequence from={SCENES[0].from} durationInFrames={SCENES[0].duration} name="S1 问题与品牌">
        <Scene1Problem />
      </Sequence>
      <Sequence from={SCENES[1].from} durationInFrames={SCENES[1].duration} name="S2 五步流程">
        <Scene2Workflow />
      </Sequence>
      <Sequence from={SCENES[2].from} durationInFrames={SCENES[2].duration} name="S3 完整走查">
        <Scene3Walkthrough />
      </Sequence>
      <Sequence from={SCENES[3].from} durationInFrames={SCENES[3].duration} name="S4 有界流式">
        <Scene4Streaming />
      </Sequence>
      <Sequence from={SCENES[4].from} durationInFrames={SCENES[4].duration} name="S5 中断恢复">
        <Scene5Recovery />
      </Sequence>
      <Sequence from={SCENES[5].from} durationInFrames={SCENES[5].duration} name="S6 总结">
        <Scene6Closing />
      </Sequence>

      {/*
        Narration is generated locally (Piper TTS, zh_CN-huayan-medium) by
        scripts/generate-narration.mjs, which reads the same cue list as the
        burned-in subtitles so picture, voice and SRT stay in sync.
      */}
      <Audio src={staticFile('audio/narration.mp3')} />

      <TransitionAccents />
      <Subtitles />
      <OpeningClosing />
    </AbsoluteFill>
  );
};
