import { useEffect, useMemo, useState } from 'react';
import { BrowserRouter, Link, Route, Routes, useNavigate, useParams } from 'react-router-dom';
import { API_BASE, createPoll, getPoll, listPolls, login, signup, submitVote } from './api';
import './App.css';

const TOKEN_KEY = 'pulsepoll-token';
const USER_KEY = 'pulsepoll-user';
const VOTER_KEY = 'pulsepoll-voter-key';
const VOTED_POLLS_KEY = 'pulsepoll-voted-polls';

function getVoterKey() {
  let voterKey = localStorage.getItem(VOTER_KEY);
  if (!voterKey) {
    voterKey = crypto.randomUUID();
    localStorage.setItem(VOTER_KEY, voterKey);
  }
  return voterKey;
}

function getVotedPolls() {
  try {
    return JSON.parse(localStorage.getItem(VOTED_POLLS_KEY) || '{}');
  } catch {
    return {};
  }
}

function hasVoted(pollID) {
  return Boolean(getVotedPolls()[pollID]);
}

function rememberVote(pollID) {
  localStorage.setItem(VOTED_POLLS_KEY, JSON.stringify({ ...getVotedPolls(), [pollID]: true }));
}

function App() {
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY) || '');
  const [user, setUser] = useState(() => {
    const saved = localStorage.getItem(USER_KEY);
    return saved ? JSON.parse(saved) : null;
  });

  const auth = useMemo(() => ({ token, user, setToken, setUser }), [token, user]);

  useEffect(() => {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  }, [token]);

  useEffect(() => {
    if (user) localStorage.setItem(USER_KEY, JSON.stringify(user));
    else localStorage.removeItem(USER_KEY);
  }, [user]);

  return (
    <BrowserRouter>
      <div className="app-shell">
        <header className="topbar">
          <Link to="/" className="brand">PulsePoll</Link>
          <nav>
            <Link to="/">Home</Link>
            <Link to="/create">Create Poll</Link>
            {!user ? (
              <>
                <Link to="/login">Login</Link>
                <Link to="/signup">Sign up</Link>
              </>
            ) : (
              <button
                type="button"
                className="ghost-btn"
                onClick={() => {
                  setToken('');
                  setUser(null);
                }}
              >
                Logout
              </button>
            )}
          </nav>
        </header>

        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/login" element={<LoginPage auth={auth} />} />
          <Route path="/signup" element={<SignupPage auth={auth} />} />
          <Route path="/create" element={<CreatePollPage auth={auth} />} />
          <Route path="/poll/:id" element={<PollRoute />} />
        </Routes>
      </div>
    </BrowserRouter>
  );
}

function PollRoute() {
  const { id } = useParams();
  return <PollPage key={id} />;
}

function HomePage() {
  const [polls, setPolls] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    async function loadPolls() {
      try {
        const data = await listPolls();
        setPolls(data);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    }

    loadPolls();
  }, []);

  return (
    <main className="page">
      <section className="hero-card">
        <div>
          <span className="eyebrow">Live polling for teams and classrooms</span>
          <h1>Build quick opinion polls and watch results update instantly.</h1>
          <p>
            Create a poll, share the link, and let your audience vote in real time with live Redis-driven updates.
          </p>
        </div>
        <div className="hero-actions">
          <Link to="/create" className="primary-btn">Create a poll</Link>
          <Link to="/signup" className="secondary-btn">Create account</Link>
        </div>
      </section>

      <section className="panel">
        <h2>Popular polls</h2>
        {loading ? <p>Loading polls...</p> : error ? <p className="error">{error}</p> : null}
        <div className="poll-grid">
          {polls.map((poll) => (
            <Link key={poll.id} className="poll-card" to={`/poll/${poll.id}`}>
              <h3>{poll.question}</h3>
              <p>{poll.options.length} options</p>
              <span>Open poll →</span>
            </Link>
          ))}
        </div>
      </section>
    </main>
  );
}

function LoginPage({ auth }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  async function handleSubmit(e) {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      const result = await login({ email, password });
      auth.setUser(result.user);
      auth.setToken(result.token);
      navigate('/create');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="page centered">
      <form className="form-panel" onSubmit={handleSubmit}>
        <h2>Login</h2>
        <input value={email} onChange={(e) => setEmail(e.target.value)} type="email" placeholder="Email" required />
        <input value={password} onChange={(e) => setPassword(e.target.value)} type="password" placeholder="Password" required />
        {error ? <p className="error">{error}</p> : null}
        <button type="submit" className="primary-btn" disabled={loading}>{loading ? 'Logging in...' : 'Login'}</button>
      </form>
    </main>
  );
}

function SignupPage({ auth }) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  async function handleSubmit(e) {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      await signup({ name, email, password });
      const result = await login({ email, password });
      auth.setUser(result.user);
      auth.setToken(result.token);
      navigate('/create');
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="page centered">
      <form className="form-panel" onSubmit={handleSubmit}>
        <h2>Create account</h2>
        <input value={name} onChange={(e) => setName(e.target.value)} type="text" placeholder="Full name" required />
        <input value={email} onChange={(e) => setEmail(e.target.value)} type="email" placeholder="Email" required />
        <input value={password} onChange={(e) => setPassword(e.target.value)} type="password" placeholder="Password" required />
        {error ? <p className="error">{error}</p> : null}
        <button type="submit" className="primary-btn" disabled={loading}>{loading ? 'Creating...' : 'Sign up'}</button>
      </form>
    </main>
  );
}

function CreatePollPage({ auth }) {
  const [question, setQuestion] = useState('');
  const [options, setOptions] = useState(['', '']);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  function updateOption(index, value) {
    setOptions((prev) => prev.map((item, i) => (i === index ? value : item)));
  }

  function addOption() {
    setOptions((prev) => (prev.length < 8 ? [...prev, ''] : prev));
  }

  async function handleSubmit(e) {
    e.preventDefault();
    setError('');

    if (!auth.token) {
      setError('Please log in first.');
      navigate('/login');
      return;
    }

    setLoading(true);
    try {
      const result = await createPoll({ question, options }, auth.token);
      navigate(`/poll/${result.id}`);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="page centered">
      <form className="form-panel large" onSubmit={handleSubmit}>
        <h2>Create a poll</h2>
        <input value={question} onChange={(e) => setQuestion(e.target.value)} placeholder="What would you like to ask?" required />
        {options.map((option, index) => (
          <input
            key={index}
            value={option}
            onChange={(e) => updateOption(index, e.target.value)}
            placeholder={`Option ${index + 1}`}
            required={index < 2}
          />
        ))}
        <div className="inline-actions">
          <button type="button" className="secondary-btn" onClick={addOption} disabled={options.length >= 8}>Add option</button>
          <button type="submit" className="primary-btn" disabled={loading}>{loading ? 'Creating...' : 'Create poll'}</button>
        </div>
        {error ? <p className="error">{error}</p> : null}
      </form>
    </main>
  );
}

function PollPage() {
  const { id } = useParams();
  const [poll, setPoll] = useState(null);
  const [selectedOption, setSelectedOption] = useState('');
  const [error, setError] = useState('');
  const [voted, setVoted] = useState(() => hasVoted(id));
  const [streamStatus, setStreamStatus] = useState('connecting');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    async function loadPoll() {
      try {
        const data = await getPoll(id);
        setPoll(data);
      } catch (err) {
        setError(err.message);
      }
    }

    if (id) loadPoll();
  }, [id]);

  useEffect(() => {
    if (!id) return;

    const source = new EventSource(`${API_BASE}/polls/${id}/stream`);
    source.onopen = () => setStreamStatus('live');
    source.onerror = () => setStreamStatus('reconnecting');
    source.addEventListener('poll-update', async () => {
      try {
        const data = await getPoll(id);
        setPoll(data);
      } catch (err) {
        setError(err.message);
      }
    });

    return () => {
      source.close();
      setStreamStatus('offline');
    };
  }, [id]);

  async function handleVote(e) {
    e.preventDefault();
    if (!selectedOption) {
      setError('Choose an option before voting.');
      return;
    }

    try {
      await submitVote(id, { optionId: selectedOption, voterKey: getVoterKey() });
      rememberVote(id);
      setVoted(true);
      const data = await getPoll(id);
      setPoll(data);
      setError('');
    } catch (err) {
      setError(err.message);
    }
  }

  if (error) return <main className="page"><div className="panel"><h2>Something went wrong</h2><p className="error">{error}</p></div></main>;
  if (!poll) return <main className="page"><div className="panel"><p>Loading poll...</p></div></main>;

  const totalVotes = poll.options.reduce((sum, option) => sum + option.votes, 0);

  async function sharePoll() {
    const url = window.location.href;
    try {
      if (navigator.share) await navigator.share({ title: poll.question, url });
      else {
        await navigator.clipboard.writeText(url);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      }
    } catch (err) {
      if (err.name !== 'AbortError') setError('Could not share this poll. Copy the URL from your browser instead.');
    }
  }

  return (
    <main className="page two-column">
      <section className="panel">
        <span className="eyebrow">Live poll</span>
        <h2>{poll.question}</h2>
        <div className="poll-meta">
          <span className={`live-indicator ${streamStatus}`}>{streamStatus === 'live' ? 'Live updates on' : 'Connecting to live updates'}</span>
          <button type="button" className="share-btn" onClick={sharePoll}>{copied ? 'Link copied' : 'Share poll'}</button>
        </div>
        <form onSubmit={handleVote}>
          {poll.options.map((option) => (
            <label key={option.id} className="vote-option">
              <input
                type="radio"
                name="poll-option"
                value={option.id}
                checked={selectedOption === option.id}
                onChange={(e) => setSelectedOption(e.target.value)}
              />
              <span>{option.text}</span>
            </label>
          ))}
          <button type="submit" className="primary-btn" disabled={voted}> {voted ? 'Vote recorded' : 'Submit vote'} </button>
        </form>
      </section>

      <section className="panel">
        <div className="results-title"><h2>Results</h2><span>{totalVotes} {totalVotes === 1 ? 'vote' : 'votes'}</span></div>
        <div className="result-list">
          {poll.options.map((option) => {
            const percentage = totalVotes === 0 ? 0 : Math.round((option.votes / totalVotes) * 100);
            return (
              <div key={option.id} className="result-row">
                <div className="result-header">
                  <span>{option.text}</span>
                  <strong>{option.votes} votes</strong>
                </div>
                <div className="progress">
                  <span style={{ width: `${percentage}%` }} />
                </div>
                <small>{percentage}%</small>
              </div>
            );
          })}
        </div>
      </section>
    </main>
  );
}

export default App;
