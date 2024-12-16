import os
import random
from datetime import datetime, timedelta

import psycopg2
from dateutil.relativedelta import relativedelta  # For adding months
from faker import Faker
from psycopg2.extras import execute_values

# Database connection settings
DB_CONFIG = {
    'dbname': os.getenv("PG_DB"),
    'user': os.getenv("PG_USER"),
    'password': os.getenv("PG_PASS"),
    'host': os.getenv("PG_HOST"),
    'port': os.getenv("PG_PORT")
}

# Initialize Faker
fake = Faker()

fund_names = [
    "roxy's dental fund", "bco general fund", "server costs",
    "whelky homecoming fund", "bco fund server costs", "gaza relief fund"
]


# Helper function to generate random dates within the past 12 months
def random_date_within_last_12_months():
    start_date = datetime.now() - timedelta(days=365)
    end_date = datetime.now()
    return fake.date_time_between(start_date=start_date, end_date=end_date)


# Helper function to add one or more months to a given date
def add_months(date, months):
    return date + relativedelta(months=months)

def calculate_next_payment_date(created_date, payout_frequency):
    today = datetime.today()
    if payout_frequency == "monthly":
        # Increment created_date until it's in the future
        next_payment = created_date
        while next_payment <= today:
            next_payment += relativedelta(months=1)
    elif payout_frequency == "once":
        # For "once", the next payment is just one month after the creation date
        next_payment = created_date + relativedelta(months=1)
    return next_payment


# Seed database
try:
    # Connect to the database
    connection = psycopg2.connect(**DB_CONFIG)
    cursor = connection.cursor()

    # Insert members with a created timestamp within the past 12 months
    members = [
        (fake.uuid4(), fake.name(), fake.email(), fake.name(), fake.name(), random_date_within_last_12_months())
        for _ in range(20)
    ]
    execute_values(cursor, "INSERT INTO member (id, bco_name, email, first_name, last_name, created) VALUES %s",
                   members)

    # Insert funds with a created timestamp within the past 12 months
    funds = []
    fund_created_map = {}

    for fund_name in fund_names:
        created_timestamp = random_date_within_last_12_months()
        payout_frequency = random.choice(["once", "monthly"])
        next_payment_timestamp = calculate_next_payment_date(created_timestamp, payout_frequency)
        expires = None
        if payout_frequency == "once":
            expires = next_payment_timestamp

        fund_id = fake.uuid4()
        funds.append((
            fund_id,
            fund_name,
            ' '.join(fake.sentences(nb=5)),
            fake.bs(),
            "paypal",
            payout_frequency,
            created_timestamp,
            next_payment_timestamp,
            expires
        ))
        fund_created_map[fund_id] = (created_timestamp, payout_frequency)

    execute_values(cursor,
                   "INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, created, next_payment, expires) VALUES %s",
                   funds)

    # Get member IDs
    cursor.execute("SELECT id FROM member")
    member_ids = [row[0] for row in cursor.fetchall()]

    # Insert donation plans
    INTERVAL_UNITS = ['WEEK', 'MONTH']
    unique_plans = {}
    NUM_PLANS = 10

    while len(unique_plans) < NUM_PLANS:
        amount_cents = random.randint(10, 100) * 100  # Random amount between $10 and $100
        interval_unit = random.choice(INTERVAL_UNITS)
        fund_id = random.choice(list(fund_created_map.keys()))

        # Ensure that funds with "once" payout frequency do not have recurring plans
        _, payout_frequency = fund_created_map[fund_id]
        if payout_frequency == "once" and interval_unit in INTERVAL_UNITS:
            continue

        if (amount_cents, interval_unit, fund_id) not in unique_plans:
            unique_plans[(amount_cents, interval_unit, fund_id)] = (
                fake.uuid4(),
                f"Plan {len(unique_plans) + 1}",
                fake.uuid4(),
                amount_cents,
                interval_unit,
                random.randint(1, 12),
                random.choice([True, False]),
                datetime.now(),
                datetime.now(),
                fund_id
            )

    donation_plans = list(unique_plans.values())
    execute_values(cursor,
                   "INSERT INTO donation_plan (id, name, paypal_plan_id, amount_cents, interval_unit, interval_count, active, created, updated, fund_id) VALUES %s",
                   donation_plans)

    # Insert donations
    donations = []
    for _ in range(500):
        recurring = False

        has_plan = random.random() < 0.8  # 80% are recurring donations
        if has_plan:
            plan = random.choice(donation_plans)
            plan_id = plan[0]
            fund_id = plan[9]
            recurring = True

        else:
            eligible_funds = [fund_id for fund_id, (_, freq) in fund_created_map.items() if freq == "once"]
            if not eligible_funds:
                continue  # No valid funds for single donations
            plan_id = None
            fund_id = random.choice(eligible_funds)

        fund_created_date, _ = fund_created_map[fund_id]
        donation_created_date = random_date_within_last_12_months()
        if donation_created_date < fund_created_date:
            donation_created_date = fund_created_date + timedelta(days=random.randint(1, 30))

        donations.append(
            (
                fake.uuid4(),
                random.choice(member_ids),
                fund_id,
                "666",
                True,
                donation_created_date,
                plan_id,
                recurring
            )
        )

    execute_values(cursor,
                   "INSERT INTO donation (id, donor_id, fund_id, provider_order_id, active, created, donation_plan_id, recurring) VALUES %s",
                   donations)

    # Generate payments
    donation_payments = []
    for donation in donations:
        donation_id, plan_id, created_date, fund_id = donation[0], donation[6], donation[5], donation[2]

        _, payout_frequency = fund_created_map[fund_id]

        if donation[7]:
            # Ensure recurring payments align with fund's payout frequency
            if payout_frequency == "once":
                continue

            cursor.execute("SELECT interval_unit, amount_cents FROM donation_plan WHERE id = %s", (plan_id,))
            plan_data = cursor.fetchone()

            if plan_data:
                interval_unit, amount_cents = plan_data

                next_payment_date = created_date
                while next_payment_date < datetime.now():
                    donation_payments.append(
                        (
                            fake.uuid4(),
                            donation_id,
                            amount_cents,
                            "paypal",
                            next_payment_date
                        )
                    )

                    if interval_unit == "WEEK":
                        next_payment_date += timedelta(weeks=1)
                    elif interval_unit == "MONTH":
                        next_payment_date = add_months(next_payment_date, 1)
        else:
            donation_payments.append(
                (
                    fake.uuid4(),
                    donation_id,
                    random.randint(10, 100) * 100,
                    "paypal",
                    created_date
                )
            )

    execute_values(cursor,
                   "INSERT INTO donation_payment (id, donation_id, amount_cents, paypal_payment_id, created) VALUES %s",
                   donation_payments)

    # Commit the transaction
    connection.commit()
    print("Database seeded successfully!")

except Exception as e:
    print(f"An error occurred: {e}")
    if connection:
        connection.rollback()

finally:
    if cursor:
        cursor.close()
    if connection:
        connection.close()
