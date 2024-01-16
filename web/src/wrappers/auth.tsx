import { Navigate, Outlet } from 'umi'

export default (props) => {
    // const { isLogin } = useAuth();
    console.log(props)
    const isLogin = false
    if (isLogin) {
        return <Outlet />;
    } else {
        return <Navigate to="/login" />;
    }
}