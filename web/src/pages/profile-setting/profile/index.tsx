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
    <section>
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
          <Input
            defaultValue="yifanwu92"
            className="bg-colors-background-inverse-weak"
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">Email</label>
          <Input
            defaultValue="yifanwu92@gmail.com"
            className="bg-colors-background-inverse-weak"
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm text-muted-foreground">Language</label>
          <Select defaultValue="english">
            <SelectTrigger className="bg-colors-background-inverse-weak">
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
            <SelectTrigger className="bg-colors-background-inverse-weak">
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
