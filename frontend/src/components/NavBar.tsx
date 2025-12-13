import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import './NavBar.css'

export function NavBar() {
  const { userToken, adminToken, logout } = useAuth()
  const location = useLocation()
  const navigate = useNavigate()

  const handleLogout = () => {
    logout()
    if (!location.pathname.startsWith('/admin')) {
      navigate('/')
    } else {
      navigate('/admin/login')
    }
  }

  const isActive = (path: string) => (location.pathname === path ? 'active' : '')

  return (
    <header className="nav">
      <div className="nav__brand">Team Invite</div>
      <nav className="nav__links">
        <Link to="/" className={isActive('/')}>抽奖大厅</Link>
        <Link to="/invite" className={isActive('/invite')}>填写邀请码</Link>
        {adminToken && <Link to="/admin" className={isActive('/admin')}>后台面板</Link>}
      </nav>
      <div className="nav__actions">
        {userToken ? (
          <button onClick={handleLogout} className="secondary">
            退出
          </button>
        ) : (
          <Link to="/login" className="primary">
            登录
          </Link>
        )}
      </div>
    </header>
  )
}
