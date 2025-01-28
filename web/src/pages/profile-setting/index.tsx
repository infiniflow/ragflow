import { Button } from '@/components/ui/button';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { ArrowLeft } from 'lucide-react';
import { Outlet } from 'umi';
import { SideBar } from './sidebar';

export default function ProfileSetting() {
  const { navigateToHome } = useNavigatePage();

  return (
    <div className="flex flex-col w-full h-screen bg-background text-foreground">
      <header className="flex items-center border-b">
        <div className="flex items-center border-r p-1.5">
          <Button variant="ghost" size="icon" onClick={navigateToHome}>
            <ArrowLeft className="w-5 h-5" />
          </Button>
        </div>
        <div className="p-4">
          <h1 className="text-2xl font-semibold tracking-tight">
            Profile & settings
          </h1>
        </div>
      </header>

      <div className="flex flex-1 bg-muted/50">
        <SideBar></SideBar>

        <main className="flex-1 ">
          <Outlet></Outlet>
          {/* <h1 className="text-3xl font-bold mb-6"> {title}</h1> */}
        </main>
      </div>
    </div>
  );
}
