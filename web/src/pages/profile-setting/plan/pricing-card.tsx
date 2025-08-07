import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { Mail, Zap } from 'lucide-react';

interface PricingFeature {
  name: string;
  value: string;
  tooltip?: string;
}

interface PricingCardProps {
  title: string;
  price: string;
  description: string;
  features: PricingFeature[];
  buttonText: string;
  buttonVariant?: 'default' | 'outline' | 'secondary';
  badge?: string;
  isPro?: boolean;
  isEnterprise?: boolean;
}

export function PricingCard({
  title,
  price,
  description,
  features,
  buttonText,
  isPro,
  isEnterprise,
}: PricingCardProps) {
  const isFree = title === 'Free';

  return (
    <Card className="flex flex-col bg-colors-background-neutral-weak divide-y divide-colors-outline-neutral-strong p-4">
      <CardHeader className=" justify-between p-0 pb-3 h-52">
        <section>
          <div className="flex items-center justify-between mb-2">
            <Badge className="text-xs">
              {isPro && <Zap className="mr-2 h-4 w-4" />}
              {isEnterprise && <Mail className="mr-2 h-4 w-4" />}
              {title}
            </Badge>
          </div>
          <p className="text-sm text-colors-text-neutral-standard">
            {description}
          </p>
        </section>
        <section>
          <div className="flex items-baseline text-3xl font-bold pb-3">
            {price}
            {price !== 'Customed' && (
              <span className="text-sm font-normal">/mo</span>
            )}
          </div>
          <Button
            className={cn('w-full', {
              'bg-colors-text-core-standard': !isFree,
            })}
            size={'sm'}
          >
            {isPro && <Zap className="mr-2 h-4 w-4" />}
            {isEnterprise && <Mail />}
            {buttonText}
          </Button>
        </section>
      </CardHeader>
      <CardContent className=" p-0 pt-3">
        <ul className="space-y-2">
          {features.map((feature, index) => (
            <li key={index} className="">
              <div className="text-colors-text-core-standard">
                {feature.name}
              </div>
              <span className="text-sm">
                <span className="font-medium">{feature.value}</span>
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
