import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

export default function Profile() {
  return (
    <section className="p-8">
      <h1 className="text-3xl font-bold mb-6">User profile</h1>
      <Avatar className="w-[120px] h-[120px] mb-6">
        <AvatarImage
          src={
            'https://gw.alipayobjects.com/zos/rmsportal/KDpgvguMpGfqaHPjicRK.svg'
          }
          alt="Profile"
        />
        <AvatarFallback>YW</AvatarFallback>
      </Avatar>

      <div className="space-y-6 max-w-[600px]">
        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">User name</label>
          <Input defaultValue="username" />
        </div>

        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">Email</label>
          <Input defaultValue="address@example.com" />
        </div>

        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">Language</label>
          <Select defaultValue="english">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="english">English</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">Timezone</label>
          <Select defaultValue="utc9">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="utc9">UTC+9 Asia/Shanghai</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <Button variant="outline" className="mt-4">
          Change password
        </Button>
      </div>
    </section>
  );
}
