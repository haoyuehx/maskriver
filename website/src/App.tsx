import Navigation from './components/Navigation';
import Hero from './components/Hero';
import VideoPlayer from './components/VideoPlayer';
import ProblemSolution from './components/ProblemSolution';
import WorkflowDemo from './components/WorkflowDemo';
import FeatureGrid from './components/FeatureGrid';
import StreamingPipeline from './components/StreamingPipeline';
import RecoveryDemo from './components/RecoveryDemo';
import Architecture from './components/Architecture';
import UseCases from './components/UseCases';
import GetStarted from './components/GetStarted';
import Faq from './components/Faq';
import Footer from './components/Footer';

export default function App() {
  return (
    <div className="site">
      <a className="skip-link" href="#main">跳到主要内容</a>
      <Navigation />
      <main id="main">
        <Hero />
        <VideoPlayer />
        <ProblemSolution />
        <WorkflowDemo />
        <FeatureGrid />
        <StreamingPipeline />
        <RecoveryDemo />
        <Architecture />
        <UseCases />
        <GetStarted />
        <Faq />
      </main>
      <Footer />
    </div>
  );
}
