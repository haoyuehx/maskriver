import React from 'react';
import {Composition, Still} from 'remotion';
import './styles/fonts';
import {EnvTest} from './scenes/EnvTest';
import {StyleTest} from './scenes/StyleTest';
import {MaskRiverVideo, SCENES} from './MaskRiverVideo';
import {Scene1Problem} from './scenes/Scene1Problem';
import {Scene2Workflow} from './scenes/Scene2Workflow';
import {Scene3Walkthrough} from './scenes/Scene3Walkthrough';
import {Scene4Streaming} from './scenes/Scene4Streaming';
import {Scene5Recovery} from './scenes/Scene5Recovery';
import {Scene6Closing} from './scenes/Scene6Closing';
import {FRAME} from './styles/tokens';

const base = {fps: FRAME.fps, width: FRAME.width, height: FRAME.height};

const SCENE_COMPONENTS = [
  Scene1Problem,
  Scene2Workflow,
  Scene3Walkthrough,
  Scene4Streaming,
  Scene5Recovery,
  Scene6Closing,
] as const;

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition id="EnvTest" component={EnvTest} durationInFrames={150} {...base} />
      <Composition id="StyleTest" component={StyleTest} durationInFrames={450} {...base} />
      <Composition id="MaskRiverDemo" component={MaskRiverVideo} durationInFrames={7800} {...base} />

      {/* Per-scene compositions: used for segment renders and frame review. */}
      {SCENES.map((scene, i) => {
        const C = SCENE_COMPONENTS[i];
        return (
          <Composition
            key={scene.id}
            id={`Dev-${i + 1}`}
            component={C}
            durationInFrames={scene.duration}
            {...base}
          />
        );
      })}

      <Still id="Poster" component={Scene1Problem} width={FRAME.width} height={FRAME.height} />
    </>
  );
};
