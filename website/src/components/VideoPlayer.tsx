import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Captions,
  ExternalLink,
  Maximize,
  Minimize,
  Pause,
  Play,
  RotateCcw,
  Volume2,
  VolumeX,
} from 'lucide-react';
import { CAPTIONS_URL, POSTER_URL, VIDEO_URL } from '../data/content';

function formatTime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
  const minutes = Math.floor(seconds / 60);
  const rest = Math.floor(seconds % 60);
  return `${minutes}:${rest.toString().padStart(2, '0')}`;
}

export default function VideoPlayer() {
  const videoRef = useRef<HTMLVideoElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const [loaded, setLoaded] = useState(false);
  const [wantPlay, setWantPlay] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(260);
  const [volume, setVolume] = useState(0.85);
  const [muted, setMuted] = useState(false);
  const [captions, setCaptions] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(max-width: 560px)').matches,
  );
  const [fullscreen, setFullscreen] = useState(false);
  const [error, setError] = useState(false);

  const loadAndPlay = useCallback(() => {
    if (error) return;
    setLoaded(true);
    setWantPlay(true);
  }, [error]);

  useEffect(() => {
    if (!loaded || !wantPlay) return;
    const video = videoRef.current;
    if (!video) return;
    void video.play().catch(() => {
      setWantPlay(false);
      setPlaying(false);
    });
    setWantPlay(false);
  }, [loaded, wantPlay]);

  useEffect(() => {
    const video = videoRef.current;
    const track = video?.textTracks?.[0];
    // The track is owned by the video element, not React state.
    // eslint-disable-next-line react-hooks/immutability
    if (track) track.mode = captions ? 'showing' : 'hidden';
  }, [captions, loaded]);

  useEffect(() => {
    const onFullscreen = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', onFullscreen);
    return () => document.removeEventListener('fullscreenchange', onFullscreen);
  }, []);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = muted ? 0 : volume;
    video.muted = muted;
  }, [volume, muted]);

  const togglePlay = () => {
    const video = videoRef.current;
    if (!loaded) {
      loadAndPlay();
      return;
    }
    if (!video) return;
    if (video.paused) {
      void video.play().catch(() => setError(true));
    } else {
      video.pause();
    }
  };

  const seek = (value: number) => {
    const video = videoRef.current;
    if (!video) return;
    video.currentTime = value;
    setCurrent(value);
  };

  const toggleCaptions = () => setCaptions((value) => !value);

  const toggleMute = () => setMuted((value) => !value);

  const toggleFullscreen = async () => {
    const stage = stageRef.current;
    if (!stage) return;
    try {
      const video = videoRef.current as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null;
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else if (stage.requestFullscreen) {
        await stage.requestFullscreen();
      } else if (video?.webkitEnterFullscreen) {
        video.webkitEnterFullscreen();
      }
    } catch {
      setFullscreen(false);
    }
  };

  const progress = duration > 0 ? (current / duration) * 100 : 0;

  return (
    <section id="demo" className="section section-shell video-section">
      <div className="section-heading">
        <span className="section-kicker">PRODUCT DEMO</span>
        <h2>4 分 20 秒，看懂 MaskRiver 的核心流程</h2>
        <p>从敏感信息识别，到安全审批、执行验证与中断恢复，直观了解它如何组织一次受控的数据脱敏任务。</p>
      </div>
      <div className={`video-frame ${fullscreen ? 'is-fullscreen' : ''}`} ref={stageRef}>
        <div className="video-stage">
          {!loaded && (
            <button type="button" className="video-poster" onClick={loadAndPlay} aria-label="播放在线演示视频">
              <img src={POSTER_URL} alt="MaskRiver 演示视频封面：数据库节点与数据流" loading="lazy" decoding="async" />
              <span className="poster-vignette" aria-hidden="true" />
              <span className="poster-copy">
                <span className="poster-label">MASK RIVER · PRODUCT FILM</span>
                <span className="poster-title">安全审批 · 可验证处理 · 故障恢复</span>
                <span className="poster-meta">04:20 · 1920×1080 · 中文画面字幕 · 无配音</span>
              </span>
              <span className="big-play" aria-hidden="true">
                <Play size={30} fill="currentColor" />
              </span>
            </button>
          )}
          {loaded && (
            <video
              ref={videoRef}
              className="demo-video"
              poster={POSTER_URL}
              preload="none"
              playsInline
              onLoadedMetadata={(event) => {
                setDuration(event.currentTarget.duration || 260);
              }}
              onTimeUpdate={(event) => setCurrent(event.currentTarget.currentTime)}
              onDurationChange={(event) => setDuration(event.currentTarget.duration || 260)}
              onPlay={() => setPlaying(true)}
              onPause={() => setPlaying(false)}
              onWaiting={() => setWaiting(true)}
              onPlaying={() => setWaiting(false)}
              onCanPlay={() => setWaiting(false)}
              onError={() => {
                setError(true);
                setPlaying(false);
              }}
              onClick={togglePlay}
              aria-label="MaskRiver 产品演示视频"
            >
              <source src={VIDEO_URL} type="video/mp4" />
              <track kind="subtitles" src={CAPTIONS_URL} srcLang="zh" label="中文" />
              您的浏览器不支持 HTML5 视频。
            </video>
          )}
          {loaded && !playing && !error && (
            <button type="button" className="video-center-play" onClick={togglePlay} aria-label="播放视频">
              <Play size={26} fill="currentColor" />
            </button>
          )}
          {waiting && <div className="video-loading" role="status">正在加载视频…</div>}
          {error && (
            <div className="video-error" role="alert">
              <strong>视频暂不可播放</strong>
              <span>请检查网络，或直接打开视频文件。</span>
              <a href={VIDEO_URL} target="_blank" rel="noreferrer">
                打开视频 <ExternalLink size={15} aria-hidden="true" />
              </a>
            </div>
          )}
          {loaded && (
            <div className="video-controls" role="group" aria-label="视频控制栏">
              <button type="button" onClick={togglePlay} aria-label={playing ? '暂停' : '播放'}>
                {playing ? <Pause size={18} fill="currentColor" /> : <Play size={18} fill="currentColor" />}
              </button>
              <span className="video-time">{formatTime(current)} / {formatTime(duration)}</span>
              <input
                className="video-progress"
                type="range"
                min={0}
                max={duration || 260}
                step={0.1}
                value={Math.min(current, duration || 260)}
                onChange={(event) => seek(Number(event.target.value))}
                aria-label="视频进度"
                aria-valuetext={`${formatTime(current)} / ${formatTime(duration)}`}
                style={{ '--progress': `${progress}%` } as React.CSSProperties}
              />
              <button type="button" onClick={toggleMute} aria-label={muted ? '取消静音' : '静音'}>
                {muted ? <VolumeX size={18} /> : <Volume2 size={18} />}
              </button>
              <input
                className="volume-slider"
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={muted ? 0 : volume}
                onChange={(event) => {
                  setVolume(Number(event.target.value));
                  setMuted(false);
                }}
                aria-label="音量"
              />
              <button
                type="button"
                className={captions ? 'is-active' : ''}
                onClick={toggleCaptions}
                aria-label={captions ? '关闭中文字幕' : '打开中文字幕'}
                aria-pressed={captions}
              >
                <Captions size={18} />
              </button>
              <button
                type="button"
                onClick={() => {
                  seek(0);
                  if (!playing) void videoRef.current?.play();
                }}
                aria-label="从头播放"
              >
                <RotateCcw size={17} />
              </button>
              <button type="button" onClick={() => void toggleFullscreen()} aria-label={fullscreen ? '退出全屏' : '全屏'}>
                {fullscreen ? <Minimize size={18} /> : <Maximize size={18} />}
              </button>
            </div>
          )}
        </div>
      </div>
      <div className="video-notes">
        <span><span className="dot teal" /> 画面内已含中文字幕，可切换外挂字幕</span>
        <span><span className="dot amber" /> 视频为无配音视觉版；中文 SRT 随站点提供</span>
        <a href={VIDEO_URL} target="_blank" rel="noreferrer">直接打开 MP4 <ExternalLink size={13} aria-hidden="true" /></a>
      </div>
    </section>
  );
}
